# Go SDK codegen (sub-plan 21.3 row C-1, Go variant)

Per-package codegen tooling decision for the Go SDK. Implementation
work (the actual scaffold + transport + tests) lands in row C-3.

## What gets generated, what stays handwritten

| Layer            | Source                                              | Style       |
| ---------------- | --------------------------------------------------- | ----------- |
| Types + stubs    | `apps/backend/openapi/openapi.yaml`                 | codegen     |
| HTTP transport   | `transport.go`                                      | handwritten |
| Webhook verifier | `webhooks/verify.go`                                | handwritten |
| x402 client      | (Phase C-7, separate module `blockchain0x-x402-go`) | handwritten |
| Resource methods | `*.go` per resource                                 | hybrid      |
| Errors           | `errors.go`                                         | handwritten |

## Tooling: oapi-codegen

Pick: [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) with
`-generate types,client` against the spec.

Rationale:

- The de facto Go OpenAPI tool. Active maintenance.
- Generates idiomatic Go types (no `interface{}` blobs).
- Client output is overridable - we keep the generated types + stub
  client and replace the transport with a handwritten one.

## Distribution

Go modules via git tags - no central registry. Each release in
`Tosh-Labs/blockchain0x-go` (the public mirror) tags `vX.Y.Z` and
`go get github.com/Tosh-Labs/blockchain0x-go@vX.Y.Z` Just Works.

## Module path

The published module path is `github.com/Tosh-Labs/blockchain0x-go`
(matches the mirror). Source-of-truth in this monorepo at
`packages/sdk-go/` does not declare a module - it's a build artifact
of the mirror pipeline, not directly `go get`-able.

## Not yet implemented

C-1 documents the decision; C-3 builds the scaffold. Until C-3 lands,
this directory is informational.
