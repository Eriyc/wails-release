---
sidebar_position: 6
---

# Frontend Runtime

This page covers the client-side integration for frontend `codepush` and `experiments`.

The runtime precedence is:

1. selected experiment
2. installed codepush
3. embedded frontend

## What clients need

Your client app needs four pieces:

- a `frontend.BundleManager`
- a `wailsupdate.Service` configured with `FrontendCatalogURL` and `FrontendCatalogPublicKey`
- `frontend.NewRuntimeFS(...)` in the Wails asset handler
- a frontend listener for `update:frontend-reload-required` that calls `window.location.reload()`

## Bundle manager

Create one `frontend.BundleManager` and reuse it for both the runtime asset handler and the updater service:

```go
frontendManager := &frontend.BundleManager{
    AppID:        "com.example.myapp",
    NativeCompat: nativeCompat,
}
```

`AppID` must match the signed frontend catalog `app_id`.

## Runtime asset handler

Replace the embedded-only asset handler:

```go
app := application.New(application.Options{
    Assets: application.AssetOptions{
        Handler: application.AssetFileServerFS(frontend.NewRuntimeFS(frontendManager, assets)),
    },
})
```

`frontend.NewRuntimeFS(...)` resolves requests against one consistent frontend layer per request, so concurrent installs and reloads do not mix files from different bundles.

## Updater service

Configure the updater service with the pinned catalog URL and Ed25519 public key:

```go
client := &http.Client{Timeout: 45 * time.Second}

service := wailsupdate.NewService(wailsupdate.Options{
    ManifestURL:               "https://releases.example.com/manifest.json",
    FrontendCatalogURL:        "https://proxy.example.com/frontend/catalog.json",
    FrontendCatalogPublicKey:  os.Getenv("FRONTEND_CATALOG_PUBLIC_KEY"),
    FrontendAutoCheck:         true,
    CurrentVersion:            appVersion,
    Channel:                   "stable",
    NativeCompat:              nativeCompat,
    TargetPath:                wailsupdate.DefaultTargetPath(),
    TempDir:                   filepath.Join(os.TempDir(), "myapp-update"),
    Client:                    client,
    FrontendManager:           frontendManager,
})
```

Rules:

- `FrontendCatalogURL` is pinned in the app config. Do not learn it from the native release manifest.
- The catalog URL must be HTTPS.
- `FrontendCatalogPublicKey` must be the Ed25519 public key for the signed catalog response.

## Register events

Register the normal updater events once:

```go
func init() {
    wailsupdate.RegisterEvents("update")
}
```

That now includes `update:frontend-reload-required`.

## Frontend reload handling

Listen for the reload event in the web app:

```js
import { Events } from "@wailsio/runtime";

Events.On("update:frontend-reload-required", () => {
    window.location.reload();
});
```

Use this same event path for:

- manual codepush apply
- forced codepush auto-apply
- experiment switch
- reset from experiment back to codepush or embedded

## Service methods

The frontend runtime uses these `wailsupdate.Service` methods:

- `RefreshFrontendCatalog()`
- `GetFrontendState()`
- `ApplyCodepush()`
- `SwitchExperiment(name string)`
- `ResetFrontend()`

Typical app-level wrappers look like:

```go
type UpdateService struct {
    service *wailsupdate.Service
}

func (s *UpdateService) GetFrontendState() wailsupdate.FrontendState {
    return s.service.GetFrontendState()
}

func (s *UpdateService) RefreshFrontendCatalog() wailsupdate.FrontendState {
    return s.service.RefreshFrontendCatalog()
}

func (s *UpdateService) ApplyCodepush() wailsupdate.ActionResponse {
    return s.service.ApplyCodepush()
}

func (s *UpdateService) SwitchExperiment(name string) wailsupdate.ActionResponse {
    return s.service.SwitchExperiment(name)
}

func (s *UpdateService) ResetFrontend() wailsupdate.ActionResponse {
    return s.service.ResetFrontend()
}
```

## Runtime behavior

### Codepush

- The client fetches the signed catalog.
- It chooses the newest compatible codepush by `PublishedAt`, then semantic version.
- If the catalog entry is `force: true`, the client attempts one automatic install for that bundle version.
- If that forced install fails, the client records one local failure for that version and does not loop forever.
- Manual `ApplyCodepush()` is still allowed later.

### Experiments

- The client lists all compatible experiments returned by the signed catalog.
- `SwitchExperiment(name)` downloads the bundle if needed, installs it, updates selection, and emits the reload event.
- `ResetFrontend()` clears the experiment selection and falls back to installed codepush or embedded assets.

### Failure handling

- If catalog fetch fails, the client keeps the current local state.
- Installed codepush and experiment bundles are not removed on catalog failure.
- If a selected experiment is removed, incompatible, or tampered locally, the client clears the selection and falls back.

## Proxy and catalog contract

Clients expect:

- `GET /frontend/catalog.json`
- HTTPS
- a signed catalog response
- codepush and experiment asset URLs already rewritten by the proxy if needed

The client verifies the final signed catalog it receives. That means proxy URL rewriting must happen before signing.

## Security checks on the client

The client runtime enforces:

- strict experiment and codepush names: lowercase `[a-z0-9-_]+`
- strict bundle checksums: `sha256:<64 hex>`
- catalog signature verification
- bundle compat checks
- installed bundle checksum verification before activation
- trusted install records for codepush and experiments
- selection validation before activating an experiment

## Minimal integration checklist

1. Pin `FrontendCatalogURL` in app config.
2. Ship the Ed25519 catalog public key with the app.
3. Create one shared `frontend.BundleManager`.
4. Serve assets with `frontend.NewRuntimeFS(frontendManager, assets)`.
5. Pass the same manager to `wailsupdate.NewService(...)`.
6. Expose the frontend service methods to the webview.
7. Reload on `update:frontend-reload-required`.
