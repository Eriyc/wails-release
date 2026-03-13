---
sidebar_position: 3
---

# CLI Commands

Every command reads `wailsrel.yaml` unless you pass `--config`.

## `init`

Create a starter config:

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
```

Use `--force` to overwrite an existing file.

## `status`

Print the resolved config path, app identifier, target hooks, frontend channels, and output directory:

```bash
wailsrel status
```

## `doctor`

Validate the local toolchain and configured build hook requirements:

```bash
wailsrel doctor
```

Run this on each CI runner before `build` or `release`.

## `bump`

Advance the application version from Git history:

```bash
wailsrel bump patch
wailsrel bump minor --prerelease
wailsrel bump pre
```

Valid bump types are `patch`, `minor`, `major`, and `pre`.

## `build`

Run the configured Wails-owned build hooks and stage discovered artifacts:

```bash
wailsrel build
```

Use `--dry-run` to print the target hook matrix without building.

## `delta`

Generate delta patches from previously published artifacts:

```bash
wailsrel delta
```

Use `delta.old_artifacts.source` to decide where old artifacts come from:

- `github-release`
- `local-cache`
- `url`

## `bundle`

Build one frontend bundle zip for the active or requested channel:

```bash
wailsrel bundle
wailsrel bundle --channel beta
```

This command uses `frontend.build_command`, `frontend.build_dir`, and computes `compat_id` from `frontend.bindings_dir`. `frontend.compat_version` is legacy warning-only config during migration.

## `compat id`

Print the deterministic compat ID derived from the frontend bindings:

```bash
wailsrel compat id
wailsrel compat id ./frontend/bindings
```

## `index release`

Emit `release-index.json` and `release-index.pb` from discovered release outputs:

```bash
wailsrel index release
```

## `index frontend`

Emit `frontend-index.json` and `frontend-index.pb` from discovered frontend bundles:

```bash
wailsrel index frontend
```

## `manifest encode`

Re-encode a release or delta manifest between JSON and protobuf:

```bash
wailsrel manifest encode --kind release --input dist/manifest.json --output dist/manifest.pb
wailsrel manifest encode --kind delta --input dist/delta-manifest.pb --output dist/delta-manifest.json
```

## `catalog verify`

Verify a signed frontend catalog:

```bash
wailsrel catalog verify --path frontend-catalog.json --public-key "$PUBLIC_KEY" --app-id com.example.myapp
```

## `channel <name>`

Set the active frontend channel stored under `.wailsrel/` for the app:

```bash
wailsrel channel stable
wailsrel channel beta
```

## `release`

Run the full release pipeline:

```bash
wailsrel release
```

`release` runs configured build hooks, stages artifacts, builds frontend bundles, generates delta patches, creates manifests, and publishes assets through the configured release provider.

## Global flags

```bash
wailsrel --config ./ci/linux.yaml --json --dry-run release
```

- `--config`, `-c`: path to `wailsrel.yaml`
- `--json`: machine-readable output
- `--dry-run`: show planned work without mutating files or publishing
- `--verbose`, `-v`: verbose logging
