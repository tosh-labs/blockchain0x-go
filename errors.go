// Package blockchain0x is the official Go SDK for the Blockchain0x
// non-custodial AI-agent wallet platform on Base.
//
// Sub-plan 21.3 row C-3. Mirrors the @blockchain0x/node + Python SDK
// error hierarchies so consumers can branch on stable wire `Code`
// strings instead of regex-matching messages.
//
//	Error                       (every SDK error)
//	  *APIKeyError              (HTTP 401/403 with apikey.*)
//	  *WebhookSignatureError    (returned by webhooks.Verify(...) when
//	                             called with the strict-error option)
//
// Every error carries:
//
//	Code        stable wire-level code from the openapi catalog
//	Message     human-readable description
//	HTTPStatus  the HTTP status that produced this error (0 for
//	            client-side raises like WebhookSignatureError)
//	RequestID   server-issued request id (empty on local-only errors)
package blockchain0x

import "fmt"

// Blockchain0xError is the base error type returned by every SDK call.
// Branch on `e.Code` (a string from the wire-level catalog) for fine-
// grained handling - never regex-match `e.Message`, which is marketing
// copy and may change.
//
// NB: the base type is named Blockchain0xError (not `Error`) on
// purpose. A type named `Error` that also has an `Error()` method
// cannot be embedded: the embedded field name (`Error`) shadows the
// promoted `Error()` method, so the outer struct would silently fail
// to satisfy the `error` interface. Naming the base type distinctly
// lets `APIKeyError`/`WebhookSignatureError` embed it and inherit
// `Error()` cleanly.
type Blockchain0xError struct {
	Code       string
	Message    string
	HTTPStatus int
	RequestID  string
}

func (e *Blockchain0xError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error is a backward-compatible alias for Blockchain0xError so that
// existing `&Error{...}` constructors and `*Error` assertions keep
// compiling. Do NOT embed `Error` (the alias) - embed the real
// `Blockchain0xError` type, otherwise the field-name collision above
// returns.
type Error = Blockchain0xError

// APIKeyError is the subtype for HTTP 401 / 403 envelopes whose
// `error.code` starts with `apikey.`.
//
// Use errors.As to extract it from a returned error:
//
//	var apikeyErr *blockchain0x.APIKeyError
//	if errors.As(err, &apikeyErr) {
//	    if apikeyErr.Code == "apikey.scope_insufficient" {
//	        // mint a fresh key with more scope
//	    }
//	}
type APIKeyError struct {
	Blockchain0xError
}

// WebhookSignatureError is returned by webhooks.Verify only when
// strict-error mode is requested. The default Verify path returns a
// VerifyResult discriminated value so callers branch on `.OK` without
// a defer/recover.
type WebhookSignatureError struct {
	Blockchain0xError
}

// IsAPIKeyError reports whether the error is an `apikey.*` envelope.
func IsAPIKeyError(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*APIKeyError); ok {
		return e.Code != ""
	}
	if e, ok := err.(*Error); ok {
		return len(e.Code) >= 7 && e.Code[:7] == "apikey."
	}
	return false
}

// Stable apikey.* failure-mode catalog (sub-plan 21.3 row C-8). The
// backend emits these codes verbatim in the wire envelope; SDK
// consumers branch on them via APIKeyError.Code. Splits into four
// groups: identity-bound, agent-flavor binding, surface-restriction,
// and the four 21.3-introduced workspace-flavor codes.
const (
	// Identity-bound (the key itself is invalid).
	CodeAPIKeyInvalid = "apikey.invalid"
	CodeAPIKeyRevoked = "apikey.revoked"
	CodeAPIKeyExpired = "apikey.expired"
	// Agent-flavor binding errors (sub-plan 21.1).
	CodeAPIKeyAgentRevoked  = "apikey.agent_revoked"
	CodeAPIKeyAgentMismatch = "apikey.agent_mismatch"
	// Surface-restriction errors.
	CodeAPIKeyWorkspaceEndpointBlocked = "apikey.workspace_endpoint_blocked"
	CodeAPIKeyUnsupportedEndpoint      = "apikey.unsupported_endpoint"
	// Network + scope.
	CodeAPIKeyNetworkMismatch   = "apikey.network_mismatch"
	CodeAPIKeyScopeInsufficient = "apikey.scope_insufficient"
	// Workspace-flavor errors (sub-plan 21.3).
	CodeAPIKeyWalletNotAssigned          = "apikey.wallet_not_assigned"
	CodeAPIKeyWorkspaceScopeInsufficient = "apikey.workspace_scope_insufficient"
	CodeAPIKeyRoleInsufficientForGrants  = "apikey.role_insufficient_for_grants"
	CodeAPIKeyNoGrantsRemaining          = "apikey.no_grants_remaining"
)
