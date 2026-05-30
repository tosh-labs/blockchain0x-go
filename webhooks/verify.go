// Package webhooks verifies inbound webhook deliveries from
// Blockchain0x.
//
// Sub-plan 21.3 row C-6 (Go variant). Ports the @blockchain0x/node +
// Python `webhooks.verify` byte-for-byte:
//
//	want = HMAC-SHA256(secret, fmt.Sprintf("%d.%s", t, rawBody))
//	ok   = subtle.ConstantTimeCompare(want, sig) && abs(now - t) <= tolerance
//
// Discriminated-union return so callers branch on `result.OK` without
// a panic/recover:
//
//	result := webhooks.Verify(webhooks.Args{
//	    Headers: r.Header, RawBody: body, Secret: secret,
//	})
//	if !result.OK {
//	    http.Error(w, result.Code, http.StatusBadRequest)
//	    return
//	}
//	// result.EventType / result.EventID / result.DeliveryID populated.
//
// The verifier accepts EITHER the structured `t=<ts>,v1=<hex>` value
// OR a bare hex signature (some load balancers strip comma-delimited
// values); when bare-hex, the `t` value is read from the
// X-Blockchain0x-Timestamp header.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Header names. HTTP headers are case-insensitive but Go's `http.Header`
// map is keyed by the canonical MIME form. The Verify entry point uses
// http.Header's `Get` method, which already canonicalises the lookup
// so a caller can pass headers straight from `r.Header` without
// massaging them.
const (
	HeaderSignature  = "X-Blockchain0x-Signature"
	HeaderTimestamp  = "X-Blockchain0x-Timestamp"
	HeaderEventType  = "X-Blockchain0x-Event-Type"
	HeaderEventID    = "X-Blockchain0x-Event-Id"
	HeaderDeliveryID = "X-Blockchain0x-Delivery-Id"

	// DefaultToleranceSeconds is the maximum drift between the
	// worker's timestamp and `time.Now()` before Verify rejects with
	// `webhook.timestamp_outside_window`. Matches the worker.
	DefaultToleranceSeconds = 300
)

// Failure-code constants mirror the @blockchain0x/node + Python
// strings verbatim so a consumer who reads the Node docs sees the
// same identifiers here.
const (
	CodeSignatureMissing       = "webhook.signature_missing"
	CodeSignatureMalformed     = "webhook.signature_malformed"
	CodeTimestampOutsideWindow = "webhook.timestamp_outside_window"
	CodeSignatureMismatch      = "webhook.signature_mismatch"
	CodeSecretMissing          = "webhook.secret_missing"
	CodeTimestampMissing       = "webhook.timestamp_missing"
	CodeTimestampInvalid       = "webhook.timestamp_invalid"
)

// Args is the input to Verify.
type Args struct {
	// Headers as they arrived from the framework. http.Header is
	// preferred; a generic map[string]string works too.
	Headers Headers
	// RawBody is the request body EXACTLY as it arrived on the wire.
	// Do not pre-parse JSON before verifying - the HMAC is over the
	// raw bytes.
	RawBody []byte
	// Secret is the workspace's webhook signing secret returned by
	// POST /v1/webhooks (shown ONCE at create-time).
	Secret string
	// ToleranceSeconds overrides DefaultToleranceSeconds. Zero falls
	// back to the default.
	ToleranceSeconds int
	// Now overrides time.Now (used for tests). Zero falls back.
	Now time.Time
}

// Headers is the minimal interface Verify needs - covers
// http.Header, http.Request.Header, and a hand-rolled map wrapper.
type Headers interface {
	Get(name string) string
}

// httpHeaderShim adapts a map[string]string to the Headers interface
// for callers who do not have an http.Header handy.
type mapHeaders map[string]string

func (m mapHeaders) Get(name string) string {
	if v, ok := m[name]; ok {
		return v
	}
	// Case-insensitive fallback. http.Header.Get canonicalises the
	// MIME form (`X-Blockchain0x-Signature`), but a plain map may
	// have a lower-cased key; check both.
	if v, ok := m[strings.ToLower(name)]; ok {
		return v
	}
	return ""
}

// HeadersFromMap wraps a `map[string]string` so callers without an
// http.Header can still use Verify.
func HeadersFromMap(m map[string]string) Headers {
	return mapHeaders(m)
}

// VerifyResult is the discriminated-union return value.
//
//   - On success: OK=true, EventType / EventID / DeliveryID are
//     populated from the corresponding headers (any can be empty
//     when the framework dropped the header but the signature
//     verified).
//   - On failure: OK=false, Code is one of the Code* constants.
type VerifyResult struct {
	OK         bool
	Code       string
	EventType  string
	EventID    string
	DeliveryID string
}

// Verify runs the HMAC + timestamp window check.
//
// Calling pattern:
//
//	r := webhooks.Verify(webhooks.Args{Headers: req.Header, RawBody: body, Secret: secret})
//	if !r.OK {
//	    http.Error(w, r.Code, http.StatusBadRequest)
//	    return
//	}
func Verify(args Args) VerifyResult {
	if args.Secret == "" {
		return fail(CodeSecretMissing)
	}
	if args.Headers == nil {
		return fail(CodeSignatureMissing)
	}
	rawSig := args.Headers.Get(HeaderSignature)
	if rawSig == "" {
		return fail(CodeSignatureMissing)
	}
	parsedTS, sigHex, ok := parseSignature(rawSig)
	if !ok {
		return fail(CodeSignatureMalformed)
	}
	ts := parsedTS
	if ts == 0 {
		raw := args.Headers.Get(HeaderTimestamp)
		if raw == "" {
			return fail(CodeTimestampMissing)
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fail(CodeTimestampInvalid)
		}
		ts = parsed
	}
	tolerance := args.ToleranceSeconds
	if tolerance <= 0 {
		tolerance = DefaultToleranceSeconds
	}
	now := args.Now
	if now.IsZero() {
		now = time.Now()
	}
	if abs64(now.Unix()-ts) > int64(tolerance) {
		return fail(CodeTimestampOutsideWindow)
	}
	mac := hmac.New(sha256.New, []byte(args.Secret))
	if _, err := fmt.Fprintf(mac, "%d.", ts); err != nil {
		return fail(CodeSignatureMismatch)
	}
	if _, err := mac.Write(args.RawBody); err != nil {
		return fail(CodeSignatureMismatch)
	}
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(sigHex))) {
		return fail(CodeSignatureMismatch)
	}
	return VerifyResult{
		OK:         true,
		EventType:  args.Headers.Get(HeaderEventType),
		EventID:    args.Headers.Get(HeaderEventID),
		DeliveryID: args.Headers.Get(HeaderDeliveryID),
	}
}

// VerifyHTTPRequest is a convenience wrapper around Verify that takes
// an *http.Request directly. It reads the body for you (and DOES NOT
// rewind it - call this BEFORE the body is consumed by a JSON
// decoder, OR pass the pre-read bytes to Verify via the Args form).
//
// Most production handlers should use Verify directly with the raw
// body they already captured (e.g. via httputil.DumpRequest or a
// reverse-proxy buffer) - this wrapper is for the single-purpose
// "verify this small payload then process" case.
func VerifyHTTPRequest(r *http.Request, secret string, body []byte) VerifyResult {
	return Verify(Args{Headers: r.Header, RawBody: body, Secret: secret})
}

func parseSignature(raw string) (timestamp int64, sigHex string, ok bool) {
	if !strings.Contains(raw, "=") {
		// Bare hex - the caller must read the timestamp from the
		// separate X-Blockchain0x-Timestamp header.
		s := strings.TrimSpace(raw)
		for _, c := range s {
			if !isHex(c) {
				return 0, "", false
			}
		}
		return 0, strings.ToLower(s), true
	}
	for _, part := range strings.Split(raw, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return 0, "", false
		}
		switch kv[0] {
		case "t":
			n, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return 0, "", false
			}
			timestamp = n
		case "v1":
			sigHex = strings.ToLower(strings.TrimSpace(kv[1]))
		}
	}
	if sigHex == "" {
		return 0, "", false
	}
	return timestamp, sigHex, true
}

func isHex(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func fail(code string) VerifyResult {
	return VerifyResult{OK: false, Code: code}
}
