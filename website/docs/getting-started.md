---
sidebar_position: 2
---

# Getting Started

## Prerequisites

- Go 1.23 or newer
- Git tags available locally if you want version discovery and changelog generation
- Wails v3 project inputs when using the build pipeline

## Quick start

Run the CLI directly from the repository while the project is still under active development:

```bash
go run ./cmd/wailsrel init
go run ./cmd/wailsrel status
go run ./cmd/wailsrel doctor
go run ./cmd/wailsrel build
go run ./cmd/wailsrel delta
```

## Reference gateways

The repository includes two release gateway implementations:

- Go gateway: `go run ./cmd/wailsrel-gateway`
- Bun gateway example: `bun run ./examples/bun-gateway.ts`

## CI baseline

The existing continuous integration workflow runs:

```bash
go test ./...
go vet ./...
```

That remains separate from the docs deployment workflow. GitHub Pages builds only the Docusaurus site in `website/`.
