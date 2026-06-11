// Sub-plan 21.3 row C-3: Go SDK module manifest.
//
// Module path matches the public mirror repo (the publish target).
// Go modules are tag-published from the mirror via git tags (e.g.
// `v0.0.1-alpha.0`); no central registry is involved.
//
// Distribution:
//
//	go get github.com/tosh-labs/blockchain0x-go@v0.0.1-alpha.0
//
// The Go ecosystem treats every module as semver, with pre-release
// suffixes following the `-alpha.N` / `-beta.N` / `-rc.N` convention.
// Setting the suffix in the tag is enough; the module manifest itself
// does not encode the version.

module github.com/tosh-labs/blockchain0x-go

go 1.22
