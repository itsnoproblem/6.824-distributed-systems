// This go.mod exists only to give content/ its own module boundary, so
// `go test ./...` run from the repo root does not try to build and test the
// exercise scaffolds under content/modules/*/exercises/* as packages of the
// main module. Those scaffolds are intentionally incomplete or broken (fix
// mode) or stubbed with TODOs (create mode) — they are meant to fail until
// a student edits them, and are loaded as plain files by internal/coursefs,
// never compiled in place. See internal/exercise/workspace.go, which
// generates its own go.mod at runtime for the actual exercise sandbox.
module contentscaffolds

go 1.25
