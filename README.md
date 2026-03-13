# wailsrel

`wailsrel` is a release and update toolkit for Wails v3 applications.

The repository currently includes the Phase 0-4 baseline:

- Cobra-based CLI with `init`, `status`, `doctor`, `bump`, `build`, `sign`, and `delta`
- YAML config loading with env expansion, defaults, and validation
- Version discovery and changelog generation from git tags
- Multi-target Wails builds with artifact staging and signing
- Delta patch generation against cached prior release artifacts
- Baseline `go test ./...` and `go vet ./...` workflow

Reference gateways now exist in both Go and Bun/WinterTC-style JavaScript:

- Go: `go run ./cmd/wailsrel-gateway`
- Bun example: `bun run ./docs/examples/bun-gateway.ts`

## Quick start

```bash
go run ./cmd/wailsrel init
go run ./cmd/wailsrel status
go run ./cmd/wailsrel doctor
go run ./cmd/wailsrel build
go run ./cmd/wailsrel delta
```
