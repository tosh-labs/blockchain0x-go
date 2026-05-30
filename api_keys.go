package blockchain0x

import (
	"context"
	"fmt"
)

// Sub-plan 21.3 row C-3 first resource: api_keys.
//
// The Go SDK exposes a thin client over the wire shape; the JSON
// responses keep the openapi field names (camelCase) so consumers
// can compare directly against the spec without translating in their
// heads. Once `codegen/regenerate.sh` lands, every body / response
// type lives under packages/sdk-go/_models/.
//
// Until then, this hand-written shape is the canonical model for the
// resource. Field set + JSON tags MUST stay in lock-step with
// apps/backend/openapi/openapi.yaml's `ApiKey` schema.

// APIKey is the persisted state of a Blockchain0x API key.
//
// Sub-plan 21.3 §1.2: the two new fields - WorkspaceScopes and
// WalletAssignments - are populated on workspace-flavor keys; both
// are empty on legacy wallet-only keys.
type APIKey struct {
	ID                string                    `json:"id"`
	Prefix            string                    `json:"prefix"`
	Label             *string                   `json:"label,omitempty"`
	Scopes            []string                  `json:"scopes,omitempty"`
	AgentID           *string                   `json:"agentId,omitempty"`
	WorkspaceScopes   []string                  `json:"workspaceScopes,omitempty"`
	WalletAssignments []APIKeyWalletAssignment  `json:"walletAssignments,omitempty"`
	CreatedAt         string                    `json:"createdAt,omitempty"`
	ExpiresAt         *string                   `json:"expiresAt,omitempty"`
	RotatedAt         *string                   `json:"rotatedAt,omitempty"`
	RevokedAt         *string                   `json:"revokedAt,omitempty"`
}

// APIKeyWalletAssignment binds a workspace-flavor key to one wallet
// with a per-wallet scope set (sub-plan 21.3 §3.3).
type APIKeyWalletAssignment struct {
	AgentID string   `json:"agentId"`
	Scopes  []string `json:"scopes"`
}

// APIKeyPage is a cursor-paginated list of API keys.
type APIKeyPage struct {
	Data []APIKey `json:"data"`
	Page PageInfo `json:"page"`
}

// PageInfo carries the cursor-paginated next/prev pointers used by
// every paginated list response in the API.
type PageInfo struct {
	Next *string `json:"next"`
	Prev *string `json:"prev"`
}

// APIKeysResource is the entry point for /v1/api-keys/* calls.
//
// Construct via `client.APIKeys`.
type APIKeysResource struct {
	client *Client
}

// List returns the workspace's active API keys.
func (r *APIKeysResource) List(ctx context.Context) (*APIKeyPage, error) {
	var out APIKeyPage
	if err := r.client.Get(ctx, "/v1/api-keys", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches one API key by id.
func (r *APIKeysResource) Get(ctx context.Context, apiKeyID string) (*APIKey, error) {
	if apiKeyID == "" {
		return nil, &Error{Code: "request.invalid", Message: "apiKeyID is required"}
	}
	var out APIKey
	if err := r.client.Get(ctx, fmt.Sprintf("/v1/api-keys/%s", apiKeyID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Revoke marks the API key as revoked. Subsequent calls authenticated
// with the key 401 with `apikey.revoked`. Idempotent.
func (r *APIKeysResource) Revoke(ctx context.Context, apiKeyID string) error {
	if apiKeyID == "" {
		return &Error{Code: "request.invalid", Message: "apiKeyID is required"}
	}
	return r.client.Delete(ctx, fmt.Sprintf("/v1/api-keys/%s", apiKeyID), RequestOptions{})
}

// APIKeyWithSecret is the response for a successful create call.
// `Secret` is the plaintext key returned ONCE; persist it now -
// the server only stores the hash. Subsequent reads return the
// stable prefix only.
type APIKeyWithSecret struct {
	APIKey
	Secret string `json:"secret"`
}

// CreateWorkspaceKeyRequest mints a sub-plan 21.3 workspace-flavor
// API key. At least one of WorkspaceScopes or WalletAssignments
// MUST be non-empty - a key with neither rejects at the server with
// `request.invalid`.
type CreateWorkspaceKeyRequest struct {
	Label             string                          `json:"label"`
	WorkspaceScopes   []string                        `json:"workspaceScopes,omitempty"`
	WalletAssignments []CreateWalletAssignmentRequest `json:"walletAssignments,omitempty"`
	// ExpiresInDays MUST be one of {7, 30, 60, 90} when set; zero
	// means "no expiry" (the server caps at 90 days for workspace
	// keys regardless).
	ExpiresInDays int `json:"expiresInDays,omitempty"`
}

// CreateWalletAssignmentRequest binds the minted key to one wallet
// with a per-wallet scope set. Mirrors the wire shape under §3.3.
type CreateWalletAssignmentRequest struct {
	AgentID string   `json:"agentId"`
	Scopes  []string `json:"scopes"`
}

// CreateAgentKeyRequest mints the legacy 21.1 agent-bound key shape.
// Distinct from the workspace flavor: ONE agentId + a flat scopes[]
// list. The two shapes are mutually exclusive on the wire; calling
// the wrong method against the wrong server payload contract
// rejects with `request.invalid`.
type CreateAgentKeyRequest struct {
	AgentID       string   `json:"agentId"`
	Label         string   `json:"label"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expiresInDays,omitempty"`
}

// CreateWorkspaceKey POSTs /v1/api-keys with the canonical
// workspace-flavor body. Returns the minted key + its plaintext
// secret. The plaintext is only ever returned by this call -
// consumers MUST persist it now or rotate the key.
//
// Server-side RBAC check fires on the wire: an over-grant (the
// minter's wallet role cannot grant the requested scope) rejects
// with 403 `apikey.role_insufficient_for_grants`. The Go client
// surfaces that as an `*APIKeyError`.
func (r *APIKeysResource) CreateWorkspaceKey(
	ctx context.Context,
	req CreateWorkspaceKeyRequest,
	opts RequestOptions,
) (*APIKeyWithSecret, error) {
	if req.Label == "" {
		return nil, &Error{Code: "request.invalid", Message: "label is required"}
	}
	if len(req.WorkspaceScopes) == 0 && len(req.WalletAssignments) == 0 {
		return nil, &Error{
			Code:    "request.invalid",
			Message: "workspace-flavor key needs at least one workspaceScope OR walletAssignment",
		}
	}
	var out APIKeyWithSecret
	if err := r.client.Post(ctx, "/v1/api-keys", req, &out, opts); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateAgentKey POSTs /v1/api-keys with the legacy agent-bound
// body. Used for the sub-plan 21.1 single-agent flow.
func (r *APIKeysResource) CreateAgentKey(
	ctx context.Context,
	req CreateAgentKeyRequest,
	opts RequestOptions,
) (*APIKeyWithSecret, error) {
	if req.AgentID == "" {
		return nil, &Error{Code: "request.invalid", Message: "agentId is required"}
	}
	if req.Label == "" {
		return nil, &Error{Code: "request.invalid", Message: "label is required"}
	}
	if len(req.Scopes) == 0 {
		return nil, &Error{
			Code:    "request.invalid",
			Message: "agent-flavor key needs at least one scope",
		}
	}
	var out APIKeyWithSecret
	if err := r.client.Post(ctx, "/v1/api-keys", req, &out, opts); err != nil {
		return nil, err
	}
	return &out, nil
}
