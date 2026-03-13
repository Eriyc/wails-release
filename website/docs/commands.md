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

Print the resolved config path, app identifier, target count, frontend channels, and output directory:

```bash
wailsrel status
```

## `doctor`

Validate the local toolchain and signing setup for the configured targets:

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

Build all configured native targets:

```bash
wailsrel build
```

Use `--dry-run` to print the target matrix without building.

## `sign <path>`

Sign one artifact with the signing configuration that matches the target OS:

```bash
wailsrel sign dist/MyApp.dmg
wailsrel sign --os windows --provider azure dist/MyApp.exe
```

OS is inferred from the file extension unless you override it.

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

This command uses `frontend.build_command`, `frontend.build_dir`, and `frontend.compat_version`.

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

`release` builds native artifacts, builds frontend bundles, generates delta patches, creates manifests, and publishes assets through the configured release provider.

## Global flags

```bash
wailsrel --config ./ci/linux.yaml --json --dry-run release
```

- `--config`, `-c`: path to `wailsrel.yaml`
- `--json`: machine-readable output
- `--dry-run`: show planned work without mutating files or publishing
- `--verbose`, `-v`: verbose logging
