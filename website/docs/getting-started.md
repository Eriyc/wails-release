---
sidebar_position: 2
---

# Getting Started

## Prerequisites

- Go 1.25 or newer
- a Wails v3 project
- Git tags available locally if `version.source: git`
- the native packaging and signing tools required by your Wails build hooks

## Install

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest
```

## Scaffold the config

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
```

`wailsrel init` creates a schema 2 `wailsrel.yaml`. Adjust the generated Wails task hooks and artifact paths to match your project.

## Minimal GitHub Releases config

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

Use `version.source: file` and set `version.file` if you do not want to derive versions from Git tags.

## Validate the environment

```bash
wailsrel status
wailsrel doctor
```

`doctor` checks the toolchain required by your configured build hooks and release provider.

## Build and publish

```bash
wailsrel build
wailsrel delta
wailsrel release
wailsrel index release
wailsrel index frontend
```

- `build` runs your configured Wails-owned build hooks and stages the discovered artifacts in `dist/`
- `delta` generates patch files and a delta manifest when `delta.enabled: true`
- `release` runs the full pipeline and publishes assets according to `release.provider`

## Switch to custom HTTP hosting

If you serve releases from your own domain, change the release and update sections:

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

That setup expects your server to expose:

- `GET /manifest`
- `GET /delta/manifest`
- `GET /frontend/catalog`
- `GET /download/{tag}/{asset_name}`

Use [Client and Hosting](./github-pages.md) for native manifest delivery, [Server Contract](./server-contract.md) for the external server rules, and [Frontend Runtime](./frontend-runtime.md) for client-side `codepush` and `experiments` integration.
