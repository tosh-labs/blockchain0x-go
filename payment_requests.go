package blockchain0x

import (
	"context"
	"errors"
	"fmt"
)

// Sub-plan 21.2 row A-3 / B-3 (Go port).
//
// `client.PaymentRequests.Settle` is the path the x402 server adapter
// calls after verifying an X-Payment header. The request body carries
// the on-chain proof tuple (txHash + payerAddress + amountUsdcVerified);
// the backend re-verifies it against the canonical `transactions`
// table before flipping the invoice to `settled`. Trust model: the
// SDK is a thin wrapper, the server is the trust anchor.
//
// No idempotency-key mint here - settle is naturally idempotent
// server-side: an invoice already in `settled` state returns 409
// `payment_request.not_settleable`, never a duplicate event.

// PaymentRequestSettleBody is the canonical settle proof tuple.
//
// Field naming + JSON tags pin the camelCase wire shape so the x402
// server middleware in packages/sdk-go-x402 can pass a directly-
// constructed value (its `SettleBody` carries identical field names).
type PaymentRequestSettleBody struct {
	TxHash             string `json:"txHash"`
	PayerAddress       string `json:"payerAddress"`
	AmountUsdcVerified string `json:"amountUsdcVerified"`
}

// PaymentRequestSettled is the server's response after a successful
// settle. The middleware only checks the call did not error; the
// fields are surfaced for handlers that want to audit metadata.
type PaymentRequestSettled struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	SettledTxHash string `json:"settledTxHash"`
	SettledAt     string `json:"settledAt"`
}

// PaymentRequestsResource is the entry point for
// /v1/payment-requests/* calls. Construct via `client.PaymentRequests`.
type PaymentRequestsResource struct {
	client *Client
}

// Settle a payment request with the on-chain proof tuple.
//
// Server validates the proof against the canonical `transactions`
// table; a tampered tuple rejects with
// `payment_request.settle_proof_invalid`. The SDK surfaces that as
// a `*Error` (NOT `*APIKeyError` - the code is not in the apikey.*
// namespace).
func (r *PaymentRequestsResource) Settle(
	ctx context.Context,
	paymentRequestID string,
	body PaymentRequestSettleBody,
) (*PaymentRequestSettled, error) {
	if paymentRequestID == "" {
		return nil, &Error{Code: "request.invalid", Message: "paymentRequestID is required"}
	}
	var out PaymentRequestSettled
	if err := r.client.Post(
		ctx,
		fmt.Sprintf("/v1/payment-requests/%s/settle", paymentRequestID),
		body,
		&out,
		RequestOptions{},
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// Sentinel that callers can match on via errors.Is in the unlikely
// event a future API surface adds a typed payment-request error
// subclass. Currently unused; kept as the stable extension point.
var errPaymentRequestSettleUnsupported = errors.New("blockchain0x: payment_requests.settle requires the workspace-flavor or agent-flavor api key with the appropriate scope")

var _ = errPaymentRequestSettleUnsupported // pin the sentinel
