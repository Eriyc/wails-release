---
sidebar_position: 2
---

# Getting Started

## What you need

- Go 1.25 or newer
- a Wails v3 project
- your normal Wails build tools and signing setup

## Install

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest
```

## Create the config

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
```

This creates a `wailsrel.yaml` file. Edit it to tell `wailsrel`:

- how to build your app
- which output files to pick up
- where updates should be served from

## Simple config example

```yaml
schema: 2

app:
  name: "MyApp"
  identifier: "com.example.myapp"

version:
  source: git
  tag_prefix: "v"

targets:
  - id: linux-amd64
    os: linux
    arch: amd64
    build:
      argv: ["task", "-t", "build/linux/Taskfile.yml", "create:appimage"]
      requires: ["task", "wails3"]
    artifacts:
      - format: appimage
        glob: "bin/*.AppImage"
      - format: deb
        glob: "bin/*.deb"

delta:
  enabled: true
  old_artifacts:
    source: github-release
    repository: "acme/myapp"

update:
  manifest_url: "https://releases.example.com/manifest"

release:
  provider: github
  github:
    repository: "acme/myapp"
```

This example says:

- build a Linux release
- look for the generated package files in `bin/`
- create update metadata
- publish the release to GitHub

If you do not want to read versions from Git tags, use `version.source: file` instead.

## How the app gets updates

Your app needs a manifest URL. The updater runtime reads that URL, checks for a newer version, and downloads the right file for the current platform.

Use `wailsupdate.NewRuntime(...)` in the app:

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
})
if err != nil {
    return err
}
```

That gives the app a simple way to check for updates.

## Check your setup

```bash
wailsrel status
wailsrel doctor
```

`doctor` checks that the tools required by your config are installed.

## Build and release

```bash
wailsrel build
wailsrel release
```

- `build` runs your configured build commands and collects the output files
- `release` runs the full pipeline and publishes the result

If delta updates are enabled, `wailsrel` creates patch files during the release flow.

## If you use your own server

If you do not want GitHub Releases as the public update endpoint, point `wailsrel` at your own server:

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

Your server should expose:

- `GET /manifest`
- `GET /delta/manifest`
- `GET /frontend/catalog`
- `GET /download/{tag}/{asset_name}`

Use [Server Contract](./server-contract.md) for the exact rules, [Client and Hosting](./github-pages.md) for hosting options, and [Frontend Runtime](./frontend-runtime.md) if you also want frontend bundle updates.
