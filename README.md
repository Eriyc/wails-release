# wailsrel

`wailsrel` is a release and update toolkit for Wails v3 applications.

The repository currently includes the Phase 0 foundation:

- Cobra-based CLI skeleton
- `wailsrel init`, `wailsrel status`, and `wailsrel doctor`
- YAML config loading with env expansion, defaults, and validation
- Shared command execution and CI-environment helpers
- Baseline `go test ./...` and `go vet ./...` workflow

## Quick start

```bash
go run ./cmd/wailsrel init
go run ./cmd/wailsrel status
go run ./cmd/wailsrel doctor
```
