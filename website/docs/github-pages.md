---
sidebar_position: 5
---

# Client and Hosting

This page covers the runtime side of `wailsrel`: where clients fetch manifests from, what routes your external server must expose, and how to wire the update packages into a Wails app.

## GitHub Releases as the origin

Use this when you want CI to publish artifacts to GitHub Releases and an external server to consume them:

```yaml
update:
  manifest_url: "https://releases.example.com/manifest"

release:
  provider: github
  github:
    repository: "acme/myapp"
```

`wailsrel release` uploads `manifest.json/.pb`, `delta-manifest.json/.pb`, and release assets to the tagged GitHub release. `wailsrel index release` and `wailsrel index frontend` emit the internal indexes your server reads.

## Custom HTTP origin

Use this when you want stable URLs on your own domain:

```yaml
update:
  manifest_url: "https://releases.example.com/manifest"

release:
  provider: http
  http:
    base_url: "https://releases.example.com"
    manifest_path: "/manifest"
    delta_manifest_path: "/delta/manifest"
    download_path_prefix: "/download"
```

Your external server must expose:

- `GET /manifest`
- `GET /delta/manifest`
- `GET /frontend/catalog`
- `GET /download/{tag}/{asset_name}`

The manifest can be public even if downloads are protected. If the client uses an authenticated `http.Client`, both manifest and asset requests can carry the same bearer token or JWT.

## Bun proxy example

The Bun example is an external JS server. Use it as infrastructure, not as part of the Go module:

```bash
cd examples/authenticated-release-proxy
export GITHUB_REPOSITORY=acme/myapp
export GITHUB_TOKEN=ghp_xxx
export PROXY_AUTH_TOKEN=change-me
bun run index.ts
```

That server should expose `/manifest`, `/delta/manifest`, `/frontend/catalog`, `/download/...`, and any auth/health routes you need.

## Preferred Wails integration

Prefer `wailsupdate.NewRuntime(...)` in Wails apps:

```go
runtime, err := wailsupdate.NewRuntime(wailsupdate.RuntimeOptions{
    AppID:          "com.example.myapp",
    CurrentVersion: appVersion,
    Channel:        "stable",
    NativeCompat:   nativeCompat,
    Source: wailsupdate.RuntimeSource{
        BaseURL: "https://releases.example.com",
    },
    Frontend: wailsupdate.RuntimeFrontend{
        CatalogPublicKey: os.Getenv("FRONTEND_CATALOG_PUBLIC_KEY"),
    },
    Client: &http.Client{Timeout: 45 * time.Second},
})
if err != nil {
    return err
}
```

That gives you:

- `runtime.Service()` for Wails service registration
- `runtime.AssetFS(assets)` for layered embedded/codepush assets
- derived `/manifest` when you supply `BaseURL`
- derived GitHub latest `manifest.json` when you supply `Repository`

## Advanced native updater integration

Use `pkg/update` directly when you need lower-level control:

```go
client := &http.Client{Timeout: 45 * time.Second}

checker := update.NewChecker(client)
applier := update.NewApplier(update.ApplierOptions{
    Client:     client,
    TargetPath: targetPath,
    TempDir:    filepath.Join(os.TempDir(), "myapp-update"),
})
```

`CurrentHash` is optional, but delta selection only works when the local executable checksum matches a published artifact.

## Frontend runtime integration

Frontend `codepush` and `experiments` use a separate signed frontend catalog served by your authenticated proxy.

Clients need:

- a pinned `FrontendCatalogURL`
- an Ed25519 `FrontendCatalogPublicKey`
- a shared runtime created with `wailsupdate.NewRuntime(...)`
- `runtime.AssetFS(...)` in the Wails asset handler
- a listener for `update:frontend-reload-required`

Use [Frontend Runtime](./frontend-runtime.md) for the complete client integration.

## Authenticated clients

If your server requires auth, attach headers in the `http.Client` transport you pass to `update.NewChecker` and `update.NewApplier`. The `examples/authenticated-http-app` sample uses a custom transport that sets `Authorization: Bearer <token>` on every updater request.

## Runtime note

Apply updates against the packaged app binary. Running the updater while the app was started with `go run` points it at a temporary Go build cache executable and is not a realistic update path.
