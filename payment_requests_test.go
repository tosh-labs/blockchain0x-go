// Sub-plan 21.2 A-3 / B-3 (Go port) tests: typed Settle body shape.
// Closes the first half of the matrix "Other resources" ❌ drift for
// Go - paymentRequests.settle is the path the x402 server adapter
// calls.

package blockchain0x

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaymentRequestsSettleSendsCanonicalBodyShape(t *testing.T) {
	var captured map[string]any
	var capturedPath, capturedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "pr_demo",
			"status": "settled",
			"settledTxHash": "0xababababababababababababababababababababababababababababababababab",
			"settledAt": "2026-05-30T12:34:56Z"
		}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	out, err := c.PaymentRequests.Settle(context.Background(), "pr_demo", PaymentRequestSettleBody{
		TxHash:             "0x" + strings.Repeat("ab", 32),
		PayerAddress:       "0x" + strings.Repeat("cd", 20),
		AmountUsdcVerified: "0.10",
	})
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("method: %q", capturedMethod)
	}
	if capturedPath != "/v1/payment-requests/pr_demo/settle" {
		t.Errorf("path: %q", capturedPath)
	}
	// Canonical camelCase wire shape.
	if captured["txHash"] != "0x"+strings.Repeat("ab", 32) {
		t.Errorf("txHash: %v", captured["txHash"])
	}
	if captured["payerAddress"] != "0x"+strings.Repeat("cd", 20) {
		t.Errorf("payerAddress: %v", captured["payerAddress"])
	}
	if captured["amountUsdcVerified"] != "0.10" {
		t.Errorf("amountUsdcVerified: %v", captured["amountUsdcVerified"])
	}

	if out.ID != "pr_demo" {
		t.Errorf("ID: %q", out.ID)
	}
	if out.Status != "settled" {
		t.Errorf("Status: %q", out.Status)
	}
	if out.SettledAt != "2026-05-30T12:34:56Z" {
		t.Errorf("SettledAt: %q", out.SettledAt)
	}
}

func TestPaymentRequestsSettleRejectsEmptyIDClientSide(t *testing.T) {
	// Empty paymentRequestID never makes it to the wire - the
	// helper raises a typed *Error before any network call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.PaymentRequests.Settle(context.Background(), "", PaymentRequestSettleBody{
		TxHash:             "0x00",
		PayerAddress:       "0x00",
		AmountUsdcVerified: "1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if e, ok := err.(*Error); !ok || e.Code != "request.invalid" {
		t.Errorf("err: want *Error code=request.invalid, got %T %v", err, err)
	}
}

func TestPaymentRequestsSettleProofInvalidReturnsBaseError(t *testing.T) {
	// A tampered proof tuple rejects with HTTP 422 +
	// payment_request.settle_proof_invalid. This is NOT an apikey.*
	// envelope - the SDK surfaces it as the base *Error, not
	// *APIKeyError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": "payment_request.settle_proof_invalid",
				"message": "txHash does not match the on-chain transfer",
				"requestId": "req_y"
			}
		}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.PaymentRequests.Settle(context.Background(), "pr_demo", PaymentRequestSettleBody{
		TxHash:             "0x00",
		PayerAddress:       "0x00",
		AmountUsdcVerified: "1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("err: want *Error, got %T", err)
	}
	if e.Code != "payment_request.settle_proof_invalid" {
		t.Errorf("err.Code: %q", e.Code)
	}
	if e.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("err.HTTPStatus: %d", e.HTTPStatus)
	}
	// IsAPIKeyError treats `apikey.*` codes specially; this code is
	// NOT in that namespace.
	if IsAPIKeyError(err) {
		t.Errorf("IsAPIKeyError should be false for payment_request.* codes")
	}
}

func TestPaymentRequestsSettleMintsIdempotencyKey(t *testing.T) {
	// Settle is naturally idempotent server-side, but the Go SDK
	// still mints an Idempotency-Key on every POST (per the
	// uniform RequestOptions path). The header should still be
	// present even though the server ignores it.
	var gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"pr_x","status":"settled"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.PaymentRequests.Settle(context.Background(), "pr_x", PaymentRequestSettleBody{
		TxHash:             "0x00",
		PayerAddress:       "0x00",
		AmountUsdcVerified: "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotIdem) < 32 {
		t.Errorf("Idempotency-Key: want >=32 chars, got %q", gotIdem)
	}
}
