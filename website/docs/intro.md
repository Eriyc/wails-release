---
slug: /
sidebar_position: 1
---

# wailsrel

`wailsrel` is for Wails consumer projects that need a release pipeline and an updater client.

It covers:

- Wails-owned build hooks for one or more OS and architecture targets
- release manifests and optional delta patch manifests
- GitHub Releases or custom HTTP delivery
- optional frontend bundles and release channels

## Pick a delivery model

- GitHub Releases: publish assets directly to a GitHub release and point clients at `https://github.com/<owner>/<repo>/releases/latest/download/manifest.json`
- Custom HTTP: publish a stable `/manifest.json`, `/delta/manifest.json`, and `/download/{tag}/{asset_name}` surface under your own domain
- Authenticated proxy: keep GitHub as the storage backend and expose a token-protected or JWT-protected HTTP facade

## Standard flow

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
wailsrel doctor
wailsrel build
wailsrel delta
wailsrel release
```

Use [Getting Started](./getting-started.md) to create the config, [CI and GitHub Actions](./ci.md) for release automation, and [Client and Hosting](./github-pages.md) for runtime integration.
