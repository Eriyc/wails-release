# wailsrel

`wailsrel` builds, signs, publishes, and applies updates for Wails v3 applications.

## Install

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest
```

## Consumer flow

1. Scaffold `wailsrel.yaml`.
2. Configure targets, signing, and release hosting.
3. Point your app at a stable `manifest.json` URL.
4. Run the release commands in CI.
5. Use `pkg/update` and optional `pkg/frontend` in the client app.

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
wailsrel doctor
wailsrel build
wailsrel delta
wailsrel release
```

## Required inputs

- GitHub publishing: `GITHUB_TOKEN`
- macOS notarization: `APPLE_ID`, `APPLE_APP_PASSWORD`, `APPLE_TEAM_ID`
- Windows Trusted Signing: `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_ENDPOINT`, `AZURE_CODE_SIGNING_NAME`, `AZURE_CERT_PROFILE`
- HTTP or authenticated delivery: a stable `manifest.json` endpoint and whatever auth token or JWT your download server expects

## Docs

- [Getting Started](./website/docs/getting-started.md)
- [CLI Commands](./website/docs/commands.md)
- [CI and GitHub Actions](./website/docs/ci.md)
- [Client and Hosting](./website/docs/github-pages.md)
