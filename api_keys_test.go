// Sub-plan 21.3 row C-3 follow-up: typed CreateWorkspaceKey +
// CreateAgentKey body-shape tests. Closes the sdk-parity-matrix
// "Workspace-flavor `create` body" 🟡 drift for Go.

package blockchain0x

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateWorkspaceKeySendsCanonicalBodyShape(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": "ak_ws_01",
			"secret": "sk_test_xxxx",
			"prefix": "sk_test_xxxx",
			"label": "Treasury",
			"workspaceScopes": ["read_workspace"],
			"walletAssignments": [{"agentId": "agw_a", "scopes": ["read_wallet_metadata"]}]
		}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	out, err := c.APIKeys.CreateWorkspaceKey(context.Background(), CreateWorkspaceKeyRequest{
		Label:           "Treasury",
		WorkspaceScopes: []string{"read_workspace"},
		WalletAssignments: []CreateWalletAssignmentRequest{
			{AgentID: "agw_a", Scopes: []string{"read_wallet_metadata"}},
		},
		ExpiresInDays: 30,
	}, RequestOptions{})
	if err != nil {
		t.Fatalf("CreateWorkspaceKey error: %v", err)
	}

	if captured["label"] != "Treasury" {
		t.Errorf("label: %v", captured["label"])
	}
	ws, _ := captured["workspaceScopes"].([]any)
	if len(ws) != 1 || ws[0] != "read_workspace" {
		t.Errorf("workspaceScopes: %v", ws)
	}
	wa, _ := captured["walletAssignments"].([]any)
	if len(wa) != 1 {
		t.Fatalf("walletAssignments: %v", wa)
	}
	assn := wa[0].(map[string]any)
	if assn["agentId"] != "agw_a" {
		t.Errorf("walletAssignments[0].agentId: %v", assn["agentId"])
	}
	scopes := assn["scopes"].([]any)
	if len(scopes) != 1 || scopes[0] != "read_wallet_metadata" {
		t.Errorf("walletAssignments[0].scopes: %v", scopes)
	}
	if captured["expiresInDays"].(float64) != 30 {
		t.Errorf("expiresInDays: %v", captured["expiresInDays"])
	}

	if out.ID != "ak_ws_01" {
		t.Errorf("response ID: %q", out.ID)
	}
	if out.Secret != "sk_test_xxxx" {
		t.Errorf("response Secret: %q", out.Secret)
	}
}

func TestCreateWorkspaceKeyOmitsEmptyOptionals(t *testing.T) {
	// A workspace-only key (no wallet assignments) should serialise
	// cleanly without `walletAssignments` or `expiresInDays` keys -
	// `omitempty` JSON tags on the request struct keep the body
	// minimal for the workspace-only shape.
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": "ak_ws_02", "secret": "sk_test_x", "prefix": "sk_test_x"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.CreateWorkspaceKey(context.Background(), CreateWorkspaceKeyRequest{
		Label:           "WS-only",
		WorkspaceScopes: []string{"manage_workspace_metadata"},
	}, RequestOptions{})
	if err != nil {
		t.Fatalf("CreateWorkspaceKey error: %v", err)
	}

	if _, ok := captured["walletAssignments"]; ok {
		t.Errorf("walletAssignments should be absent: %v", captured)
	}
	if _, ok := captured["expiresInDays"]; ok {
		t.Errorf("expiresInDays should be absent: %v", captured)
	}
	ws, _ := captured["workspaceScopes"].([]any)
	if len(ws) != 1 || ws[0] != "manage_workspace_metadata" {
		t.Errorf("workspaceScopes: %v", ws)
	}
}

func TestCreateWorkspaceKeyClientValidatesEmptyGrants(t *testing.T) {
	// A workspace-flavor key with NEITHER workspaceScopes NOR
	// walletAssignments is a programming bug - the client rejects
	// before issuing a wire call, with the same error code the
	// server would have returned anyway.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.CreateWorkspaceKey(context.Background(), CreateWorkspaceKeyRequest{
		Label: "broken",
	}, RequestOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if e, ok := err.(*Error); !ok || e.Code != "request.invalid" {
		t.Errorf("err: want *Error code=request.invalid, got %T %v", err, err)
	}
}

func TestCreateWorkspaceKeyRejectionReturnsAPIKeyError(t *testing.T) {
	// Server rejects an over-grant with 403 +
	// apikey.role_insufficient_for_grants. The client surfaces it
	// as a typed *APIKeyError so callers can branch on .Code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"apikey.role_insufficient_for_grants","message":"Viewer cannot grant manage_workspace_metadata","requestId":"req_x"}}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.CreateWorkspaceKey(context.Background(), CreateWorkspaceKeyRequest{
		Label:           "over-grant",
		WorkspaceScopes: []string{"manage_workspace_metadata"},
	}, RequestOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	apikeyErr, ok := err.(*APIKeyError)
	if !ok {
		t.Fatalf("want *APIKeyError, got %T", err)
	}
	if apikeyErr.Code != CodeAPIKeyRoleInsufficientForGrants {
		t.Errorf("err.Code: want %q, got %q", CodeAPIKeyRoleInsufficientForGrants, apikeyErr.Code)
	}
	if apikeyErr.HTTPStatus != http.StatusForbidden {
		t.Errorf("err.HTTPStatus: want 403, got %d", apikeyErr.HTTPStatus)
	}
}

func TestCreateAgentKeySendsLegacyBodyShape(t *testing.T) {
	// Agent-bound keys (sub-plan 21.1) use the agentId + flat
	// scopes[] body shape, NEVER mixed with workspaceScopes /
	// walletAssignments.
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": "ak_a_01", "secret": "sk_test_a", "prefix": "sk_test_a"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.CreateAgentKey(context.Background(), CreateAgentKeyRequest{
		AgentID: "agw_a",
		Label:   "agent A",
		Scopes:  []string{"pay_bills", "read_wallet_metadata"},
	}, RequestOptions{})
	if err != nil {
		t.Fatalf("CreateAgentKey error: %v", err)
	}

	if captured["agentId"] != "agw_a" {
		t.Errorf("agentId: %v", captured["agentId"])
	}
	if captured["label"] != "agent A" {
		t.Errorf("label: %v", captured["label"])
	}
	scopes := captured["scopes"].([]any)
	if len(scopes) != 2 || scopes[0] != "pay_bills" || scopes[1] != "read_wallet_metadata" {
		t.Errorf("scopes: %v", scopes)
	}
	if _, ok := captured["workspaceScopes"]; ok {
		t.Errorf("workspaceScopes should be absent: %v", captured)
	}
	if _, ok := captured["walletAssignments"]; ok {
		t.Errorf("walletAssignments should be absent: %v", captured)
	}
}

func TestCreateWorkspaceKeyRespectsIdempotencyKeyOverride(t *testing.T) {
	var gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"ak_x","secret":"x","prefix":"x"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(Options{APIKey: "sk_test_x", BaseURL: srv.URL})
	_, err := c.APIKeys.CreateWorkspaceKey(context.Background(), CreateWorkspaceKeyRequest{
		Label:           "explicit-idem",
		WorkspaceScopes: []string{"read_workspace"},
	}, RequestOptions{IdempotencyKey: "my-replay-key"})
	if err != nil {
		t.Fatal(err)
	}
	if gotIdem != "my-replay-key" {
		t.Errorf("Idempotency-Key: want my-replay-key, got %q", gotIdem)
	}
}
