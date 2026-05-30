package blockchain0x

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Sub-plan 21.3 row C-3 client tests. Cover the operational paths that
// the @blockchain0x/node + Python SDKs are tested against: header
// injection, idempotency-key minting, network inference, error
// envelope mapping (apikey.* vs other), retry on 5xx.

func TestNewClientRequiresAPIKey(t *testing.T) {
	if _, err := NewClient(Options{APIKey: ""}); err == nil {
		t.Fatal("expected error for empty APIKey")
	}
}

func TestPinsNetworkFromSKTestPrefix(t *testing.T) {
	var gotNetwork, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNetwork = r.Header.Get("X-Network")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"page":{"next":null,"prev":null}}`))
	}))
	defer srv.Close()

	c, err := NewClient(Options{APIKey: "sk_test_demo", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.APIKeys.List(context.Background()); err != nil {
		t.Fatalf("List error: %v", err)
	}
	if gotNetwork != "testnet" {
		t.Errorf("X-Network: want testnet, got %q", gotNetwork)
	}
	if gotAuth != "Bearer sk_test_demo" {
		t.Errorf("Authorization: want Bearer sk_test_demo, got %q", gotAuth)
	}
}

func TestPinsNetworkFromSKLivePrefix(t *testing.T) {
	var gotNetwork string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNetwork = r.Header.Get("X-Network")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"page":{"next":null,"prev":null}}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_live_demo", BaseURL: srv.URL})
	if _, err := c.APIKeys.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotNetwork != "mainnet" {
		t.Errorf("X-Network: want mainnet, got %q", gotNetwork)
	}
}

func TestPostMintsIdempotencyKey(t *testing.T) {
	var gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	if err := c.Post(context.Background(), "/v1/api-keys", map[string]any{}, nil, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(gotIdem) < 32 {
		t.Errorf("Idempotency-Key: want >=32 chars, got %q", gotIdem)
	}
}

func TestPostRespectsCallerSuppliedIdempotencyKey(t *testing.T) {
	var gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	err := c.Post(context.Background(), "/v1/api-keys", map[string]any{}, nil, RequestOptions{
		IdempotencyKey: "my-stable-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotIdem != "my-stable-key" {
		t.Errorf("Idempotency-Key: want my-stable-key, got %q", gotIdem)
	}
}

func TestErrorEnvelopeAPIKeyCodeReturnsAPIKeyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"apikey.scope_insufficient","message":"wrong scope","requestId":"req_demo"}}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.List(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apikeyErr, ok := err.(*APIKeyError)
	if !ok {
		t.Fatalf("want *APIKeyError, got %T", err)
	}
	if apikeyErr.Code != "apikey.scope_insufficient" {
		t.Errorf("Code: %q", apikeyErr.Code)
	}
	if apikeyErr.HTTPStatus != http.StatusForbidden {
		t.Errorf("HTTPStatus: %d", apikeyErr.HTTPStatus)
	}
	if !IsAPIKeyError(err) {
		t.Error("IsAPIKeyError should return true")
	}
}

func TestErrorEnvelopeNonAPIKeyCodeReturnsBaseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"wallet.not_found","message":"missing"}}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.Get(context.Background(), "ak_x")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*APIKeyError); ok {
		t.Fatal("want *Error, got *APIKeyError")
	}
	if e, ok := err.(*Error); !ok || e.Code != "wallet.not_found" {
		t.Errorf("unexpected error: %#v", err)
	}
}

func TestRetriesOn503ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"internal.unavailable","message":"x"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"page":{"next":null,"prev":null}}`))
	}))
	defer srv.Close()

	prevSleep := sleep
	sleep = func(d time.Duration) {} // skip backoff in tests
	defer func() { sleep = prevSleep }()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL, MaxRetries: 3})
	if _, err := c.APIKeys.List(context.Background()); err != nil {
		t.Fatalf("List error: %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls: want 3, got %d", calls)
	}
}

func TestNonRetryable4xxDoesNotRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"request.invalid","message":"x"}}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL, MaxRetries: 3})
	_, err := c.APIKeys.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls: want 1, got %d", calls)
	}
}

// Round-trip test: the API key list shape decodes correctly,
// including the new workspaceScopes + walletAssignments fields the
// sub-plan 21.3 §3 added on the workspace flavor.
func TestAPIKeyDecodesWorkspaceFlavorFields(t *testing.T) {
	page := APIKeyPage{
		Data: []APIKey{
			{
				ID:              "ak_ws_1",
				Prefix:          "sk_test_abc",
				WorkspaceScopes: []string{"read_workspace"},
				WalletAssignments: []APIKeyWalletAssignment{
					{AgentID: "agt_a", Scopes: []string{"read_wallet_metadata"}},
				},
			},
		},
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"workspaceScopes":["read_workspace"]`) {
		t.Errorf("serialised page missing workspaceScopes: %s", body)
	}
	var decoded APIKeyPage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Data) != 1 {
		t.Fatal("want 1 row")
	}
	if decoded.Data[0].WorkspaceScopes[0] != "read_workspace" {
		t.Errorf("WorkspaceScopes did not round-trip: %v", decoded.Data[0].WorkspaceScopes)
	}
	if len(decoded.Data[0].WalletAssignments) != 1 ||
		decoded.Data[0].WalletAssignments[0].AgentID != "agt_a" {
		t.Errorf("WalletAssignments did not round-trip: %+v", decoded.Data[0].WalletAssignments)
	}
}
