---
sidebar_position: 4
---

# CI and GitHub Actions

## Using `wailsrel` in CI

`wailsrel` is GitHub Actions aware:

- `build` and `release` write step outputs when `GITHUB_OUTPUT` is available
- `release` publishes GitHub release assets with `GITHUB_TOKEN`
- `delta` reuses `GITHUB_TOKEN` when it needs to fetch old artifacts from GitHub releases

To keep those GitHub Actions outputs enabled, leave this on in `wailsrel.yaml`:

```yaml
ci:
  provider: github
  artifacts:
    upload: true
```

## Recommended workflow shape

`wailsrel` runs every target listed in the selected config file. In CI, keep each job aligned with the targets that runner can actually satisfy. For multi-platform releases, use one job or config file per runner instead of asking one runner to build every OS.

This example shows a Linux release job:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release-linux:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Install wailsrel
        run: go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest

      - name: Install Wails
        run: go install github.com/wailsapp/wails/v3/cmd/wails3@latest

      - name: Validate environment
        run: wailsrel --config ci/linux.yaml doctor

      - name: Publish release
        id: release
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: wailsrel --config ci/linux.yaml release

      - name: Upload staged artifacts
        if: ${{ steps.release.outputs.artifact_count != '0' }}
        uses: actions/upload-artifact@v4
        with:
          name: wailsrel-linux-${{ steps.release.outputs.version }}
          path: |
            ${{ steps.release.outputs.artifact_dir }}
            ${{ steps.release.outputs.manifest_path }}
```

## Required env and secrets

`wailsrel` still expands `${NAME}` placeholders from the environment when it loads YAML config, but native signing and packaging credentials now belong to your Wails project and its `build/` tooling.

In practice:

- `wailsrel` itself needs `GITHUB_TOKEN` for GitHub Releases publishing and for `delta.old_artifacts.source: github-release`
- your Wails tasks may need additional secrets, certificates, SDK paths, or signing credentials
- those native inputs must be provisioned before `doctor`, `build`, or `release`, because `wailsrel` now invokes your configured Wails-owned hooks rather than signing or packaging itself

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## GitHub Actions step outputs

When `build` or `release` runs inside GitHub Actions, `wailsrel` writes these outputs to the current step:

- `artifact_dir`: directory containing the staged files for `actions/upload-artifact`
- `artifact_count`: number of staged artifacts
- `version`: resolved release version
- `manifest_path`: generated release manifest path

`release` populates all four values. `build` mainly exposes `artifact_dir` and `artifact_count`; the version and manifest path may be empty.

## CI notes

- Use `actions/checkout` with `fetch-depth: 0` when `version.source: git` depends on local tags.
- Set `GITHUB_TOKEN` for `release` jobs, otherwise publishing to GitHub releases will fail.
- If you do not want GitHub Actions outputs or staged upload directories, set `ci.artifacts.upload: false`.
- Run `doctor` on each runner before `build` or `release` so missing Wails toolchain requirements fail early.
