# wailsrel

`wailsrel` orchestrates releases and updates for Wails v3 applications.

## Install

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest
```

## Consumer flow

1. Scaffold `wailsrel.yaml`.
2. Configure Wails-owned build hooks, artifact discovery, and release hosting.
3. Publish `manifest.json/.pb`, `delta-manifest.json/.pb`, `release-index.json/.pb`, and `frontend-index.json/.pb` from CI.
4. Run an external HTTP server that reads those published artifacts and serves `/manifest`, `/delta/manifest`, `/frontend/catalog`, and `/download/:tag/:asset_name`.
5. Use `pkg/update`, `pkg/wailsupdate`, and optional `pkg/frontend` in the client app.

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
wailsrel doctor
wailsrel compat id
wailsrel build
wailsrel delta
wailsrel release
wailsrel index release
wailsrel index frontend
```

## Required inputs

- GitHub publishing: `GITHUB_TOKEN`
- Native signing credentials and platform packaging config: managed by your Wails project and its `build/` tooling
- HTTP or authenticated delivery: an external server that rewrites published index artifacts into `/manifest`, `/delta/manifest`, `/frontend/catalog`, and `/download/...`

## Docs

- [Getting Started](./website/docs/getting-started.md)
- [CLI Commands](./website/docs/commands.md)
- [CI and GitHub Actions](./website/docs/ci.md)
- [Client and Hosting](./website/docs/github-pages.md)
- [Server Contract](./website/docs/server-contract.md)
- [JS Server Example](./website/docs/js-server-example.md)
- [Frontend Runtime](./website/docs/frontend-runtime.md)
