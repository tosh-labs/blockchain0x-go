package blockchain0x

import (
	"context"
	"errors"
)

// Sub-plan 21.1 row B-4 (Go port).
//
// `client.Payments.Create` is the agent's outbound-spend path. Two
// safety differences from every other SDK call:
//
//   1. **Auto Idempotency-Key.** The Go Client.do() path mints one on
//      every POST anyway; this resource exposes an explicit
//      RequestOptions.IdempotencyKey knob so consumers can thread a
//      stable value across retries.
//   2. **Retry stays at zero attempts by default.** A payment retry
//      without a matching idempotency key could double-submit. The
//      Client's general retry policy still applies via the
//      mintIdempotencyKey path, but consumers who do not want the
//      server-side collapse should set MaxRetries=0 at construction.
//
// The Node SDK has a per-call retry override (off | default); Go
// folds the equivalent control into the existing Client.MaxRetries
// + RequestOptions.IdempotencyKey surface.

// PaymentCreateRequest is the body for POST /v1/payments.
//
// JSON tags pin the canonical camelCase wire shape so the x402
// client wrapper (which builds the body via this struct) and a
// hand-rolled `client.Post(ctx, "/v1/payments", ...)` agree
// byte-for-byte.
type PaymentCreateRequest struct {
	AgentID   string                 `json:"agentId"`
	To        string                 `json:"to"`
	AmountWei string                 `json:"amountWei"`
	Token     string                 `json:"token,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Payment is the persisted state of an outbound spend.
type Payment struct {
	ID          string                 `json:"id"`
	AgentID     string                 `json:"agentId"`
	To          *string                `json:"to,omitempty"`
	AmountWei   string                 `json:"amountWei"`
	Status      string                 `json:"status"`
	TxHash      *string                `json:"txHash,omitempty"`
	Network     string                 `json:"network"`
	CreatedAt   string                 `json:"createdAt,omitempty"`
	CompletedAt *string                `json:"completedAt,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PaymentsResource is the entry point for /v1/payments/* calls.
type PaymentsResource struct {
	client *Client
}

// Create submits an outbound payment.
//
// Server-side errors flow through the canonical envelope mapping:
// `apikey.scope_insufficient` -> *APIKeyError, every other code
// (including the payment-specific `payment.amount_zero` etc.) ->
// *Error.
func (r *PaymentsResource) Create(
	ctx context.Context,
	req PaymentCreateRequest,
	opts RequestOptions,
) (*Payment, error) {
	if req.AgentID == "" {
		return nil, &Error{Code: "request.invalid", Message: "agentId is required"}
	}
	if req.To == "" {
		return nil, &Error{Code: "request.invalid", Message: "to is required"}
	}
	if req.AmountWei == "" {
		return nil, &Error{Code: "request.invalid", Message: "amountWei is required"}
	}
	var out Payment
	if err := r.client.Post(ctx, "/v1/payments", req, &out, opts); err != nil {
		return nil, err
	}
	return &out, nil
}

// errPaymentNoRetryOverride is reserved for the case where a future
// version of the Go SDK exposes a per-call retry: off / default
// switch matching the Node SDK's PaymentCreateOptions.retry knob.
// Currently the Go client's MaxRetries field is the equivalent
// global control.
var errPaymentNoRetryOverride = errors.New("blockchain0x: per-call retry override not yet supported on Go; configure MaxRetries on the Client")

var _ = errPaymentNoRetryOverride // pin the sentinel
