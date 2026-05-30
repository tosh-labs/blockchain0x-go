// Sub-plan 21.1 B-4 + 21.2 B-3 Go ports tests. Pins typed
// Payments.Create + Transactions.Get body shapes against
// httptest fixtures.

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

func TestPaymentsCreateSendsCanonicalBodyShape(t *testing.T) {
	var captured map[string]any
	var capturedMethod, capturedPath, capturedIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedIdem = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"pay_01","agentId":"agw_a","amountWei":"100000","status":"pending","network":"testnet"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	out, err := c.Payments.Create(context.Background(), PaymentCreateRequest{
		AgentID:   "agw_a",
		To:        "0xrecipient",
		AmountWei: "100000",
		Metadata:  map[string]interface{}{"invoice": "inv_01"},
	}, RequestOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("method: %q", capturedMethod)
	}
	if capturedPath != "/v1/payments" {
		t.Errorf("path: %q", capturedPath)
	}
	if len(capturedIdem) < 32 {
		t.Errorf("Idempotency-Key: want >=32 chars, got %q", capturedIdem)
	}
	if captured["agentId"] != "agw_a" {
		t.Errorf("agentId: %v", captured["agentId"])
	}
	if captured["to"] != "0xrecipient" {
		t.Errorf("to: %v", captured["to"])
	}
	if captured["amountWei"] != "100000" {
		t.Errorf("amountWei: %v", captured["amountWei"])
	}
	meta := captured["metadata"].(map[string]any)
	if meta["invoice"] != "inv_01" {
		t.Errorf("metadata.invoice: %v", meta["invoice"])
	}
	if out.ID != "pay_01" {
		t.Errorf("ID: %q", out.ID)
	}
	if out.Status != "pending" {
		t.Errorf("Status: %q", out.Status)
	}
}

func TestPaymentsCreateOmitsEmptyOptionalsViaOmitempty(t *testing.T) {
	// Without token / metadata, the json:"...,omitempty" tags drop
	// those keys from the wire body.
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"pay_02","amountWei":"1"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.Payments.Create(context.Background(), PaymentCreateRequest{
		AgentID: "agw_a", To: "0xZ", AmountWei: "1",
	}, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := captured["token"]; ok {
		t.Errorf("token should be absent: %v", captured)
	}
	if _, ok := captured["metadata"]; ok {
		t.Errorf("metadata should be absent: %v", captured)
	}
}

func TestPaymentsCreateRejectsMissingRequiredFieldsClientSide(t *testing.T) {
	// Each missing-required-field case raises BEFORE any wire call.
	// httptest.requestCount-equivalent: handler fails the test if hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.Payments.Create(context.Background(), PaymentCreateRequest{
		To: "0xZ", AmountWei: "1",
	}, RequestOptions{})
	if err == nil {
		t.Fatal("expected error on empty AgentID")
	}
	if e, ok := err.(*Error); !ok || e.Code != "request.invalid" {
		t.Errorf("err: %T %v", err, err)
	}

	_, err = c.Payments.Create(context.Background(), PaymentCreateRequest{
		AgentID: "agw_a", AmountWei: "1",
	}, RequestOptions{})
	if err == nil {
		t.Fatal("expected error on empty To")
	}

	_, err = c.Payments.Create(context.Background(), PaymentCreateRequest{
		AgentID: "agw_a", To: "0xZ",
	}, RequestOptions{})
	if err == nil {
		t.Fatal("expected error on empty AmountWei")
	}
}

func TestPaymentsCreateRespectsIdempotencyKeyOverride(t *testing.T) {
	var gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"pay_03","amountWei":"1"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.Payments.Create(context.Background(), PaymentCreateRequest{
		AgentID: "agw_a", To: "0xZ", AmountWei: "1",
	}, RequestOptions{IdempotencyKey: "my-replay-key"})
	if err != nil {
		t.Fatal(err)
	}
	if gotIdem != "my-replay-key" {
		t.Errorf("Idempotency-Key: want my-replay-key, got %q", gotIdem)
	}
}

func TestTransactionsGetHitsTheRightPath(t *testing.T) {
	var capturedMethod, capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"tx_01",
			"agentId":"agw_a",
			"direction":"outbound",
			"status":"confirmed",
			"txHash":"0x` + strings.Repeat("ab", 32) + `",
			"amountWei":"100000",
			"network":"testnet"
		}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	out, err := c.Transactions.Get(context.Background(), "tx_01")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if capturedMethod != "GET" {
		t.Errorf("method: %q", capturedMethod)
	}
	if capturedPath != "/v1/transactions/tx_01" {
		t.Errorf("path: %q", capturedPath)
	}
	if out.Status != "confirmed" {
		t.Errorf("Status: %q", out.Status)
	}
	if out.Direction != "outbound" {
		t.Errorf("Direction: %q", out.Direction)
	}
}

func TestTransactionsGetRejectsEmptyIDClientSide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.Transactions.Get(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if e, ok := err.(*Error); !ok || e.Code != "request.invalid" {
		t.Errorf("err: want *Error code=request.invalid, got %T %v", err, err)
	}
}
