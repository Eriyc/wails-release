---
sidebar_position: 5
---

# Client and Hosting

This page covers the runtime side of `wailsrel`: where clients fetch manifests from, what routes your server must expose, and how to wire the update packages into a Wails app.

## GitHub Releases as the origin

Use this when you want the updater to read directly from GitHub:

```yaml
update:
  manifest_url: "https://github.com/acme/myapp/releases/latest/download/manifest.json"

release:
  provider: github
  github:
    repository: "acme/myapp"
```

`wailsrel release` uploads `manifest.json`, any delta manifest, and all release assets to the tagged GitHub release. Clients can keep using the stable `releases/latest/download/manifest.json` URL.

## Custom HTTP origin

Use this when you want stable URLs on your own domain:

```yaml
update:
  manifest_url: "https://releases.example.com/manifest.json"

release:
  provider: http
  http:
    base_url: "https://releases.example.com"
    manifest_path: "/manifest.json"
    delta_manifest_path: "/delta/manifest.json"
    download_path_prefix: "/download"
```

Your server must expose:

- `GET /manifest.json`
- `GET /delta/manifest.json`
- `GET /download/{tag}/{asset_name}`

The manifest can be public even if downloads are protected. If the client uses an authenticated `http.Client`, both manifest and asset requests can carry the same bearer token or JWT.

## Go gateway

The Go gateway is a JWT-protected facade over GitHub Releases:

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel-gateway@latest
wailsrel-gateway
```

Required environment variables:

- `WAILSREL_GATEWAY_GITHUB_REPOSITORY`
- `WAILSREL_GATEWAY_GITHUB_TOKEN`
- `WAILSREL_GATEWAY_JWKS_URL`
- `WAILSREL_GATEWAY_JWT_ISSUER`
- `WAILSREL_GATEWAY_JWT_AUDIENCE`

Optional:

- `WAILSREL_GATEWAY_ADDR` default `:8080`
- `WAILSREL_GATEWAY_GITHUB_API_BASE_URL` default `https://api.github.com`

Routes:

- `GET /manifest.json`
- `GET /delta/manifest.json`
- `GET /download/{tag}/{asset_name}`

Every request requires `Authorization: Bearer <jwt>`.

## Bun proxy example

The Bun example is simpler if you only need a static bearer token:

```bash
cd examples/authenticated-release-proxy
export GITHUB_REPOSITORY=acme/myapp
export GITHUB_TOKEN=ghp_xxx
export PROXY_AUTH_TOKEN=change-me
bun run index.ts
```

That proxy exposes the same three updater routes plus `GET /healthz`.

## Native updater integration

Use `pkg/update` in your Wails app:

```go
client := &http.Client{Timeout: 45 * time.Second}

checker := update.NewChecker(client)
applier := update.NewApplier(update.ApplierOptions{
    Client:     client,
    TargetPath: targetPath,
    TempDir:    filepath.Join(os.TempDir(), "myapp-update"),
})

manager := update.NewManager(update.ManagerOpts{
    Checker: checker,
    Applier: applier,
    CheckOpts: update.CheckOpts{
        CurrentVersion: appVersion,
        CurrentHash:    currentHash,
        Channel:        "stable",
        NativeCompat:   "1",
        ManifestURL:    "https://releases.example.com/manifest.json",
    },
})
```

`CurrentHash` is optional, but delta selection only works when the local executable checksum matches a published artifact.

## Frontend bundle integration

If you ship frontend bundles by channel, add a `frontend.BundleManager` to the applier:

```go
bundles := &frontend.BundleManager{
    AppID:        "com.example.myapp",
    NativeCompat: "1",
}

applier := update.NewApplier(update.ApplierOptions{
    Client:          client,
    FrontendManager: bundles,
    TargetPath:      targetPath,
    TempDir:         filepath.Join(os.TempDir(), "myapp-update"),
})
```

Use `wailsrel bundle` to create bundle zips and `wailsrel channel <name>` to switch the active channel on the client machine.

## Authenticated clients

If your server requires auth, attach headers in the `http.Client` transport you pass to `update.NewChecker` and `update.NewApplier`. The `examples/authenticated-http-app` sample uses a custom transport that sets `Authorization: Bearer <token>` on every updater request.

## Runtime note

Apply updates against the packaged app binary. Running the updater while the app was started with `go run` points it at a temporary Go build cache executable and is not a realistic update path.
