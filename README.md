# blockchain0x-go

[![Go reference](https://pkg.go.dev/badge/github.com/Tosh-Labs/blockchain0x-go.svg)](https://pkg.go.dev/github.com/Tosh-Labs/blockchain0x-go)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go ≥ 1.22](https://img.shields.io/badge/go-%E2%89%A51.22-brightgreen.svg)](#requirements)

**Official Go SDK for [Blockchain0x](https://blockchain0x.com)** -
the non-custodial AI-agent wallet platform on Base. Pre-release: the
scaffold ships the operational essentials - HTTP transport, retry +
idempotency, `apiKeys` resource, and `webhooks.Verify`. Full resource
surface lands in sub-plan 21.3 Phase C follow-up rows.

## Install

```bash
go get github.com/Tosh-Labs/blockchain0x-go@v0.0.1-alpha.0
```

Requires Go 1.22 or newer.

## Quick start

```go
package main

import (
	"context"
	"log"

	blockchain0x "github.com/Tosh-Labs/blockchain0x-go"
)

func main() {
	client, err := blockchain0x.NewClient(blockchain0x.Options{APIKey: "sk_test_..."})
	if err != nil {
		log.Fatal(err)
	}
	page, err := client.APIKeys.List(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, k := range page.Data {
		log.Printf("%s\t%s", k.ID, k.Prefix)
	}
}
```

The client pins the network from the API-key prefix (`sk_test_*` →
testnet, `sk_live_*` → mainnet). Override with
`blockchain0x.Options{Network: blockchain0x.NetworkMainnet}` when you
need both modes in one process.

## Verify webhook signatures

The single most important utility this SDK ships - drop it into the
top of your webhook handler BEFORE touching the body.

```go
import (
	"io"
	"net/http"

	"github.com/Tosh-Labs/blockchain0x-go/webhooks"
)

func receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	result := webhooks.Verify(webhooks.Args{
		Headers: r.Header,
		RawBody: body,
		Secret:  os.Getenv("BLOCKCHAIN0X_WEBHOOK_SECRET"),
	})
	if !result.OK {
		http.Error(w, result.Code, http.StatusBadRequest)
		return
	}
	// result.EventType / result.EventID / result.DeliveryID populated.
	processEvent(body, result.EventType)
}
```

The verifier:

- Reads `X-Blockchain0x-Signature` in either `t=<ts>,v1=<hex>` or
  bare-hex form (some load balancers strip commas).
- Falls back to `X-Blockchain0x-Timestamp` when the signature is bare.
- Rejects with `webhook.timestamp_outside_window` when drift exceeds
  300 seconds (5-minute replay window; matches the worker).
- Constant-time compares via `hmac.Equal`.

## Errors

Two types:

- `*blockchain0x.Error` - base type; every SDK error returns this or a subtype.
- `*blockchain0x.APIKeyError` - subtype for HTTP 401 / 403 envelopes whose `error.code` starts with `apikey.`.

Always branch on `Code`, never regex-match `Message`:

```go
import (
	"errors"

	blockchain0x "github.com/Tosh-Labs/blockchain0x-go"
)

_, err := client.APIKeys.List(ctx)
var apikeyErr *blockchain0x.APIKeyError
if errors.As(err, &apikeyErr) {
	if apikeyErr.Code == "apikey.scope_insufficient" {
		// mint a fresh key with more scope
	}
}
```

The package-level helper `blockchain0x.IsAPIKeyError(err)` returns
true when the error originated from an `apikey.*` envelope.

## Retry behaviour

The transport retries on `429` and `5xx` with exponential backoff
(500ms, 1s, 2s, …, capped at 30s; 3 retries by default). `Retry-After`
is honoured when the server sends it.

`POST` / `PATCH` / `DELETE` requests carry an `Idempotency-Key` header

- the SDK mints a v4 UUID if you do not supply one. Pass
  `blockchain0x.RequestOptions{IdempotencyKey: "..."}` to thread a stable
  key across SDK retries OR across processes (e.g. a cron job that
  hashes its input deterministically).

## Workspace keys (sub-plan 21.3)

Two key shapes exist (see
[docs/concept-api-key-types.md](https://github.com/Tosh-Labs/blockchain0x-app/blob/dev/docs/concept-api-key-types.md)
for the full decision tree):

- **Wallet-only** - bound to ONE agent. Right shape for an autonomous AI agent that IS one wallet.
- **Workspace** - human-operator key that can carry workspace-level scopes AND assignments to N specific wallets.

The Go SDK exposes both shapes through `APIKey.Scopes`,
`APIKey.WorkspaceScopes`, and `APIKey.WalletAssignments`. The response
JSON field names mirror the OpenAPI spec exactly.

Server-side RBAC: the minter cannot grant a scope they do not have
themselves. Over-grants reject with `apikey.role_insufficient_for_grants`
which is surfaced as a `*blockchain0x.APIKeyError`.

## x402 (Phase C-7)

The sibling module `blockchain0x-x402-go` will ship the x402 client +
net/http middleware in sub-plan 21.3 row C-7. The wire format is
identical across languages so a Go service can accept payments from a
Node client and vice-versa.

## Codegen

Type-only types are generated from
`apps/backend/openapi/openapi.yaml` via
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) -
see [codegen/README.md](./codegen/README.md) for the decision
rationale.

## Source-of-truth + distribution

Source-of-truth: this directory in
[Tosh-Labs/blockchain0x-app](https://github.com/Tosh-Labs/blockchain0x-app)
under `packages/sdk-go/`.

Public mirror: [Tosh-Labs/blockchain0x-go](https://github.com/Tosh-Labs/blockchain0x-go)
(receives merges from this directory on dispatch of the
`mirror-sdk-go` workflow). Go modules read tags from this mirror
directly - no central registry is involved.

## License

Apache-2.0.
