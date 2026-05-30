// Sub-plan 21.3 row C-6 (Go) verifier tests. Mirror the Python
// failure-mode coverage so the Node, Python, and Go SDKs agree byte-
// for-byte on what a valid signature looks like.

package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

const testSecret = "whsec_test_super_secret_value"

func sign(t *testing.T, ts int64, body string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testSecret))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func headers(pairs ...string) Headers {
	if len(pairs)%2 != 0 {
		panic("headers takes name/value pairs")
	}
	m := map[string]string{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return HeadersFromMap(m)
}

// ----------------------------- happy path -----------------------------

func TestOKRoundTrip(t *testing.T) {
	body := `{"event":"payment.received"}`
	ts := time.Now().Unix()
	hdrs := headers(
		HeaderSignature, sign(t, ts, body),
		HeaderEventType, "payment.received",
		HeaderEventID, "evt_01",
		HeaderDeliveryID, "webhook_w1",
	)
	r := Verify(Args{Headers: hdrs, RawBody: []byte(body), Secret: testSecret})
	if !r.OK {
		t.Fatalf("expected ok, got %+v", r)
	}
	if r.EventType != "payment.received" {
		t.Errorf("EventType: want payment.received, got %q", r.EventType)
	}
	if r.EventID != "evt_01" {
		t.Errorf("EventID: want evt_01, got %q", r.EventID)
	}
	if r.DeliveryID != "webhook_w1" {
		t.Errorf("DeliveryID: want webhook_w1, got %q", r.DeliveryID)
	}
}

func TestOKBareHexWithSeparateTimestamp(t *testing.T) {
	body := `{"a":1}`
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(testSecret))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	hdrs := headers(
		HeaderSignature, hex.EncodeToString(mac.Sum(nil)),
		HeaderTimestamp, fmt.Sprintf("%d", ts),
	)
	r := Verify(Args{Headers: hdrs, RawBody: []byte(body), Secret: testSecret})
	if !r.OK {
		t.Fatalf("expected ok, got %+v", r)
	}
}

// ----------------------------- failure modes -----------------------------

func TestFailSignatureMissing(t *testing.T) {
	r := Verify(Args{Headers: headers(), RawBody: []byte("{}"), Secret: testSecret})
	if r.OK || r.Code != CodeSignatureMissing {
		t.Fatalf("want %s, got %+v", CodeSignatureMissing, r)
	}
}

func TestFailSignatureMalformed(t *testing.T) {
	hdrs := headers(HeaderSignature, "garbage=")
	r := Verify(Args{Headers: hdrs, RawBody: []byte("{}"), Secret: testSecret})
	if r.OK || r.Code != CodeSignatureMalformed {
		t.Fatalf("want %s, got %+v", CodeSignatureMalformed, r)
	}
}

func TestFailSecretMissing(t *testing.T) {
	body := "{}"
	ts := time.Now().Unix()
	hdrs := headers(HeaderSignature, sign(t, ts, body))
	r := Verify(Args{Headers: hdrs, RawBody: []byte(body), Secret: ""})
	if r.OK || r.Code != CodeSecretMissing {
		t.Fatalf("want %s, got %+v", CodeSecretMissing, r)
	}
}

func TestFailTimestampMissingWhenBareHex(t *testing.T) {
	// 32 hex chars: parses as bare-hex; no Timestamp header -> miss.
	hdrs := headers(HeaderSignature, "abcdef0123456789abcdef0123456789")
	r := Verify(Args{Headers: hdrs, RawBody: []byte("{}"), Secret: testSecret})
	if r.OK || r.Code != CodeTimestampMissing {
		t.Fatalf("want %s, got %+v", CodeTimestampMissing, r)
	}
}

func TestFailTimestampInvalid(t *testing.T) {
	hdrs := headers(
		HeaderSignature, "abcdef0123456789abcdef0123456789",
		HeaderTimestamp, "not-a-number",
	)
	r := Verify(Args{Headers: hdrs, RawBody: []byte("{}"), Secret: testSecret})
	if r.OK || r.Code != CodeTimestampInvalid {
		t.Fatalf("want %s, got %+v", CodeTimestampInvalid, r)
	}
}

func TestFailTimestampOutsideWindow(t *testing.T) {
	body := "{}"
	oldTS := time.Now().Unix() - 10*60 // 10 minutes old
	hdrs := headers(HeaderSignature, sign(t, oldTS, body))
	r := Verify(Args{Headers: hdrs, RawBody: []byte(body), Secret: testSecret})
	if r.OK || r.Code != CodeTimestampOutsideWindow {
		t.Fatalf("want %s, got %+v", CodeTimestampOutsideWindow, r)
	}
}

func TestFailSignatureMismatchWrongSecret(t *testing.T) {
	body := "{}"
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte("OTHER_SECRET"))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	hdrs := headers(
		HeaderSignature,
		fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil))),
	)
	r := Verify(Args{Headers: hdrs, RawBody: []byte(body), Secret: testSecret})
	if r.OK || r.Code != CodeSignatureMismatch {
		t.Fatalf("want %s, got %+v", CodeSignatureMismatch, r)
	}
}

func TestFailSignatureMismatchTamperedBody(t *testing.T) {
	original := `{"amount":1}`
	tampered := `{"amount":1000}`
	ts := time.Now().Unix()
	hdrs := headers(HeaderSignature, sign(t, ts, original))
	r := Verify(Args{Headers: hdrs, RawBody: []byte(tampered), Secret: testSecret})
	if r.OK || r.Code != CodeSignatureMismatch {
		t.Fatalf("want %s, got %+v", CodeSignatureMismatch, r)
	}
}

// ----------------------------- now override -----------------------------

func TestNowOverridePassesWithPinnedTimestamp(t *testing.T) {
	body := "{}"
	pinnedTS := int64(1_700_000_000)
	hdrs := headers(HeaderSignature, sign(t, pinnedTS, body))
	r := Verify(Args{
		Headers: hdrs,
		RawBody: []byte(body),
		Secret:  testSecret,
		Now:     time.Unix(pinnedTS+30, 0),
	})
	if !r.OK {
		t.Fatalf("expected ok with Now override, got %+v", r)
	}
}
