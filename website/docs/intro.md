---
slug: /
sidebar_position: 1
---

# wailsrel

`wailsrel` is for Wails consumer projects that need a release pipeline and an updater client.

It covers:

- Wails-owned build hooks for one or more OS and architecture targets
- protobuf-first release contracts with JSON twins
- GitHub Releases as artifact storage plus an external HTTP server
- optional frontend bundles and release channels

## Pick a delivery model

- GitHub Releases: publish assets and indexes to a tagged release
- Custom HTTP: expose `/manifest`, `/delta/manifest`, `/frontend/catalog`, and `/download/{tag}/{asset_name}` from your own server
- Authenticated proxy: keep GitHub as storage and expose a token-protected or JWT-protected HTTP facade that reads the published indexes

## Standard flow

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

Use [Getting Started](./getting-started.md) to create the config, [CI and GitHub Actions](./ci.md) for release automation, [Client and Hosting](./github-pages.md) for native manifest delivery, [Server Contract](./server-contract.md) for the external server rules, and [Frontend Runtime](./frontend-runtime.md) for frontend `codepush` and `experiments`.
