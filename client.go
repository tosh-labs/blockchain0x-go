package blockchain0x

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sub-plan 21.3 row C-3: thin sync HTTP client over net/http.
//
// Behaviour mirrors the @blockchain0x/node + Python SDKs:
//
//   - injects the Bearer + X-Network headers on every request,
//   - mints an Idempotency-Key on POST/PATCH/DELETE unless caller
//     supplies one,
//   - retries 5xx + 429 with exponential backoff (Retry-After honoured),
//   - converts the canonical error envelope into *APIKeyError / *Error.
//
// Network selection:
//   - `sk_test_*` keys force testnet,
//   - `sk_live_*` keys force mainnet,
//   - explicit Network option overrides the key prefix when the SDK is
//     used in mixed-mode tests.

const (
	defaultBaseURL    = "https://api.blockchain0x.com"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	userAgent         = "blockchain0x-go/0.0.1-alpha.0"
)

// Network is the per-request network gate; the api-key-auth plugin
// rejects mismatches with `apikey.network_mismatch`.
type Network string

const (
	NetworkMainnet Network = "mainnet"
	NetworkTestnet Network = "testnet"
)

// Options configures a Client.
type Options struct {
	APIKey     string
	BaseURL    string
	Network    Network // empty -> inferred from APIKey prefix
	Timeout    time.Duration
	MaxRetries int
	HTTPClient *http.Client // optional override (defaults to a tuned client)
}

// Client is the top-level resource entry point.
//
// Construct via NewClient(opts). The Client is safe for concurrent use
// by multiple goroutines.
type Client struct {
	apiKey     string
	baseURL    string
	network    Network
	maxRetries int
	httpClient *http.Client

	APIKeys         *APIKeysResource
	PaymentRequests *PaymentRequestsResource
	Payments        *PaymentsResource
	Transactions    *TransactionsResource
}

// NewClient constructs a Client. Returns an error when APIKey is
// empty.
func NewClient(opts Options) (*Client, error) {
	if opts.APIKey == "" {
		return nil, errors.New("blockchain0x: APIKey is required")
	}
	c := &Client{
		apiKey:     opts.APIKey,
		baseURL:    valueOrDefault(opts.BaseURL, defaultBaseURL),
		network:    networkOrInfer(opts.Network, opts.APIKey),
		maxRetries: nonZero(opts.MaxRetries, defaultMaxRetries),
	}
	if opts.HTTPClient != nil {
		c.httpClient = opts.HTTPClient
	} else {
		c.httpClient = &http.Client{Timeout: valueOrZero(opts.Timeout, defaultTimeout)}
	}
	c.APIKeys = &APIKeysResource{client: c}
	c.PaymentRequests = &PaymentRequestsResource{client: c}
	c.Payments = &PaymentsResource{client: c}
	c.Transactions = &TransactionsResource{client: c}
	return c, nil
}

// RequestOptions are per-call overrides.
type RequestOptions struct {
	// IdempotencyKey overrides the auto-minted UUID for POST/PATCH/
	// DELETE. Threading a stable value across retries OR across
	// processes (e.g. a cron job that hashes its input deterministically)
	// keeps the server-side dedupe correct.
	IdempotencyKey string
	// Network overrides the client-default network for this single
	// call. Useful when a test fixture mixes sk_test + sk_live keys.
	Network Network
}

func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out, RequestOptions{})
}

func (c *Client) Post(ctx context.Context, path string, body any, out any, opts RequestOptions) error {
	return c.do(ctx, http.MethodPost, path, body, out, opts)
}

func (c *Client) Patch(ctx context.Context, path string, body any, out any, opts RequestOptions) error {
	return c.do(ctx, http.MethodPatch, path, body, out, opts)
}

func (c *Client) Delete(ctx context.Context, path string, opts RequestOptions) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil, opts)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, opts RequestOptions) error {
	url := strings.TrimRight(c.baseURL, "/") + path
	network := opts.Network
	if network == "" {
		network = c.network
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return &Error{Code: "request.invalid", Message: fmt.Sprintf("encode body: %v", err)}
		}
	}
	needsIdempotency := method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete
	idem := opts.IdempotencyKey
	if needsIdempotency && idem == "" {
		idem = newUUID()
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxRetries+1; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return &Error{Code: "request.invalid", Message: err.Error()}
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("X-Network", string(network))
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if needsIdempotency {
			req.Header.Set("Idempotency-Key", idem)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt > c.maxRetries {
				return &Error{Code: "network.unavailable", Message: err.Error()}
			}
			sleep(backoff(attempt))
			continue
		}

		if resp.StatusCode < 400 {
			defer resp.Body.Close()
			if out == nil || resp.StatusCode == http.StatusNoContent {
				_, _ = io.Copy(io.Discard, resp.Body)
				return nil
			}
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
				return &Error{Code: "response.invalid_envelope", Message: err.Error()}
			}
			return nil
		}

		// Retryable: 429 + 5xx.
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			if attempt > c.maxRetries {
				return c.errorForResponse(resp)
			}
			wait := backoff(attempt)
			if v := resp.Header.Get("Retry-After"); v != "" {
				if secs, err := strconv.Atoi(v); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			resp.Body.Close()
			sleep(wait)
			continue
		}
		// Non-retryable.
		return c.errorForResponse(resp)
	}
	if lastErr != nil {
		return &Error{Code: "network.unavailable", Message: lastErr.Error()}
	}
	return &Error{Code: "network.unavailable", Message: "retry budget exhausted"}
}

func (c *Client) errorForResponse(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	requestID := resp.Header.Get("x-request-id")
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error.Code == "" {
		return &Error{
			Code:       "response.invalid_envelope",
			Message:    fmt.Sprintf("non-canonical error body (status=%d)", resp.StatusCode),
			HTTPStatus: resp.StatusCode,
			RequestID:  requestID,
		}
	}
	base := Error{
		Code:       envelope.Error.Code,
		Message:    envelope.Error.Message,
		HTTPStatus: resp.StatusCode,
		RequestID:  requestID,
	}
	if strings.HasPrefix(envelope.Error.Code, "apikey.") {
		return &APIKeyError{Blockchain0xError: base}
	}
	return &base
}

func backoff(attempt int) time.Duration {
	d := 500 * time.Millisecond * time.Duration(1<<(attempt-1))
	const cap = 30 * time.Second
	if d > cap {
		return cap
	}
	return d
}

func networkOrInfer(n Network, apiKey string) Network {
	if n != "" {
		return n
	}
	if strings.HasPrefix(apiKey, "sk_live_") {
		return NetworkMainnet
	}
	return NetworkTestnet
}

func valueOrDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func valueOrZero(v, fallback time.Duration) time.Duration {
	if v == 0 {
		return fallback
	}
	return v
}

func nonZero(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func newUUID() string {
	// 16 random bytes -> RFC 4122 v4 UUID. crypto/rand + manual byte
	// manipulation keeps us off any third-party dep for this single
	// function. The hex output (32 chars) is also acceptable on the
	// wire; the server only checks for the Idempotency-Key string,
	// not the exact format.
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is exotic; fall back to a timestamp-
		// derived stand-in so the request still ships.
		return fmt.Sprintf("ik-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// sleep is a variable to let tests stub the wait without waiting in
// real time. Production calls time.Sleep.
var sleep = time.Sleep
