package blockchain0x

import (
	"context"
	"fmt"
)

// Sub-plan 21.2 row B-3 (Go port).
//
// Read-only handle on the `transactions` table. The x402 client polls
// `Transactions.Get` to find out when a freshly broadcast
// `Payments.Create` has confirmed on-chain (status flips to
// `confirmed`). Scope: `read_wallet_metadata`.

// Transaction is one row in the transactions ledger.
type Transaction struct {
	ID          string  `json:"id"`
	AgentID     string  `json:"agentId"`
	Direction   string  `json:"direction"`
	Status      string  `json:"status"`
	TxHash      *string `json:"txHash,omitempty"`
	UserOpHash  *string `json:"userOpHash,omitempty"`
	FromAddress *string `json:"fromAddress,omitempty"`
	ToAddress   *string `json:"toAddress,omitempty"`
	AmountWei   string  `json:"amountWei"`
	Token       string  `json:"token,omitempty"`
	Network     string  `json:"network"`
	CreatedAt   string  `json:"createdAt,omitempty"`
	ConfirmedAt *string `json:"confirmedAt,omitempty"`
}

// TransactionsResource is the entry point for /v1/transactions/* calls.
type TransactionsResource struct {
	client *Client
}

// Get fetches one transaction by id. Rejects empty ids client-side
// with `request.invalid` rather than letting them produce a 404.
func (r *TransactionsResource) Get(ctx context.Context, transactionID string) (*Transaction, error) {
	if transactionID == "" {
		return nil, &Error{Code: "request.invalid", Message: "transactionID is required"}
	}
	var out Transaction
	if err := r.client.Get(ctx, fmt.Sprintf("/v1/transactions/%s", transactionID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
