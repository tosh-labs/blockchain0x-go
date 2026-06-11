# Releasing `blockchain0x-go` (Go SDK)

Sub-plan 21.3 row G-1 (Go variant). 2-step procedure for publishing a
new version of the Go SDK. Go modules are tag-distributed - no
separate publish step is needed.

## Prerequisites (one-time, owner)

1. **Public mirror repo**: create `tosh-labs/blockchain0x-go`
   (Apache-2.0; default branch `main`). The first mirror snapshot
   supplies the README/license.

2. **GitHub PAT**: same fine-grained PAT used by the Node + Python
   mirror workflows (Contents: read+write on the mirror repos). The
   secret `MIRROR_TO_PUBLIC_GITHUB_PAT_TOKEN` on
   `tosh-labs/blockchain0x-app` covers all three.

3. **No third-party registry**: Go modules read straight from git
   tags on the public mirror. No Trusted Publisher OIDC setup.
   `pkg.go.dev` discovers the module automatically on the first
   tag push.

## Release flow

### Step 1 - merge your changes into `dev`

Make whatever changes you need under `packages/sdk-go/`. Run the unit
suite locally to catch regressions before the mirror runs them on CI:

```bash
cd packages/sdk-go
go test ./...
```

Commit + push to `dev` like any other change. Unlike npm / PyPI, the
Go SDK does not carry a version field in `go.mod` - the version comes
from the git tag set by the mirror workflow.

### Step 2 - dispatch the mirror workflow

From the GitHub Actions tab on `tosh-labs/blockchain0x-app`:

1. Open the `mirror-sdk-go` workflow.
2. Click **Run workflow** -> branch `dev`.
3. Fill in `version` with a valid Go semver tag (e.g.
   `v0.0.1-alpha.0`, `v0.1.0`, `v1.2.3-rc.4`). The leading `v` is
   MANDATORY - Go module tag resolution rejects bare-number tags.
4. The default `dry_run=false` is correct for a real release; toggle
   `dry_run=true` first if you want to preview the staged snapshot.
5. The workflow:
   - validates the version is a proper Go semver tag,
   - runs `go test ./...` against the source under
     `packages/sdk-go/`,
   - stages the snapshot,
   - replaces the public repo's contents,
   - commits, tags `<version>`, and pushes.

`go get github.com/tosh-labs/blockchain0x-go@<version>` is live as
soon as the push lands (~10 seconds). `pkg.go.dev` indexes the new
version on its next sweep (usually within an hour).

## Verify the release

```bash
mkdir /tmp/b0x-verify && cd /tmp/b0x-verify
go mod init verify
go get github.com/tosh-labs/blockchain0x-go@v0.0.1-alpha.0
go doc github.com/tosh-labs/blockchain0x-go
```

The `go doc` output should show the package summary + the public
types (Client, APIKey, etc.).

## Pre-release versioning

Go semver pre-release tags follow the `-alpha.N` / `-beta.N` /
`-rc.N` convention. `go get` resolves them only when explicitly
requested - `go get github.com/tosh-labs/blockchain0x-go@latest`
prefers stable releases first.

Examples:

- `v0.0.1-alpha.0` - first alpha
- `v0.0.1-alpha.1` - second alpha
- `v0.0.1-beta.0` - first beta
- `v0.0.1-rc.0` - first release candidate
- `v0.0.1` - stable release

## Rollback

A pushed tag CAN be deleted (`git tag -d v0.0.1-alpha.0 && git push
origin :refs/tags/v0.0.1-alpha.0`) but Go's module proxy
(proxy.golang.org) caches the module bytes indefinitely under the
tag, so consumers who downloaded that exact version keep getting it.
Treat tags as immutable in practice: a bad release gets a fresh
patch tag (e.g. `v0.0.1-alpha.1`) instead of a re-tag of the same
version.

## Cross-references

- [packages/sdk-node/RELEASING.md](../sdk-node/RELEASING.md) - npm flow.
- [packages/sdk-python/RELEASING.md](../sdk-python/RELEASING.md) - PyPI flow.
- [docs/concept-api-key-types.md](../../docs/concept-api-key-types.md) - SDK surface decision tree.
- [.github/workflows/mirror-sdk-go.yml](../../.github/workflows/mirror-sdk-go.yml) - the mirror workflow source.
