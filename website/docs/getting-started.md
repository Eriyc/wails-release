---
sidebar_position: 2
---

# Getting Started

## Prerequisites

- Go 1.25 or newer
- a Wails v3 project
- Git tags available locally if `version.source: git`
- the signing tools required by the targets in your config

## Install

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest
```

## Scaffold the config

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
```

`wailsrel init` creates a full `wailsrel.yaml`. Delete the targets you do not build and replace the placeholder signing values.

## Minimal GitHub Releases config

```yaml
app:
  name: "MyApp"
  identifier: "com.example.myapp"

version:
  source: git
  tag_prefix: "v"

targets:
  - os: linux
    arch: [amd64]
    output_formats: [appimage, deb]
    sign:
      provider: none

delta:
  enabled: true
  old_artifacts:
    source: github-release
    repository: "acme/myapp"

update:
  manifest_url: "https://github.com/acme/myapp/releases/latest/download/manifest.json"

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

`doctor` checks the toolchain for the configured targets and verifies that the selected signing provider is usable on the current machine or CI runner.

## Build and publish

```bash
wailsrel build
wailsrel delta
wailsrel release
```

- `build` creates the configured native artifacts in `dist/`
- `delta` generates patch files and a delta manifest when `delta.enabled: true`
- `release` runs the full pipeline and publishes assets according to `release.provider`

## Switch to custom HTTP hosting

If you serve releases from your own domain, change the release and update sections:

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

That setup expects your server to expose:

- `GET /manifest.json`
- `GET /delta/manifest.json`
- `GET /download/{tag}/{asset_name}`

Use [Client and Hosting](./github-pages.md) for the gateway and updater integration details.
