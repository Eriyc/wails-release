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

## Required env and secrets by target

`wailsrel` expands `${NAME}` placeholders from the environment when it loads YAML config. That makes it straightforward to keep signing credentials in GitHub Actions secrets or variables instead of committing them to `wailsrel.yaml`.

Any job that runs `doctor`, `build`, or `release` for a signed target needs the same signing env and prepared files available before that step runs.

### Linux (`sign.provider: none`)

- `GITHUB_TOKEN` to publish GitHub release assets

Keep `GITHUB_TOKEN` available if `delta.old_artifacts.source: github-release` needs to fetch prior release assets during delta generation.

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### macOS (`sign.provider: apple`)

`wailsrel` requires these values when notarization is enabled:

- `APPLE_ID`
- `APPLE_APP_PASSWORD`
- `APPLE_TEAM_ID`

`sign.identity` is also required, but it is usually the Developer ID certificate name rather than a secret. The matching certificate and private key must already be present in the runner keychain before `doctor`, `build`, or `release` runs. If you import that certificate inside CI, store the bundle and its import password as GitHub secrets and install it in a prior step.

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  APPLE_ID: ${{ secrets.APPLE_ID }}
  APPLE_APP_PASSWORD: ${{ secrets.APPLE_APP_PASSWORD }}
  APPLE_TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
```

### macOS (`sign.provider: apple-rcodesign`)

`wailsrel` reads file paths for `apple-rcodesign`, so the workflow needs to materialize secret files before it invokes `doctor`, `build`, or `release`.

Required inputs:

- `RCODESIGN_P12_FILE` pointing to the PKCS#12 signing bundle
- `RCODESIGN_P12_PASSWORD` or `RCODESIGN_P12_PASSWORD_FILE` if the bundle is password protected
- `RCODESIGN_API_KEY_FILE` when `notarize: true`

The usual pattern is to store the PKCS#12 bundle, its password, and the App Store Connect API key in GitHub secrets, write them to temporary files in an earlier step, and then export the file paths for `wailsrel`.

### Windows (`sign.provider: azure`)

Azure Trusted Signing requires both signing configuration and Azure service principal credentials.

Required values:

- `AZURE_TENANT_ID`
- `AZURE_CLIENT_ID`
- `AZURE_CLIENT_SECRET`
- `AZURE_ENDPOINT`
- `AZURE_CODE_SIGNING_NAME`
- `AZURE_CERT_PROFILE`

Optional when the DLL is not installed in the default Program Files location:

- `AZURE_TRUSTED_SIGNING_DLIB`

The scaffolded config template already expects `AZURE_ENDPOINT`, `AZURE_CODE_SIGNING_NAME`, and `AZURE_CERT_PROFILE` from the environment, so you can keep them in GitHub Actions variables or secrets and pass the Azure credentials as secrets.

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  AZURE_TENANT_ID: ${{ secrets.AZURE_TENANT_ID }}
  AZURE_CLIENT_ID: ${{ secrets.AZURE_CLIENT_ID }}
  AZURE_CLIENT_SECRET: ${{ secrets.AZURE_CLIENT_SECRET }}
  AZURE_ENDPOINT: ${{ vars.AZURE_ENDPOINT }}
  AZURE_CODE_SIGNING_NAME: ${{ vars.AZURE_CODE_SIGNING_NAME }}
  AZURE_CERT_PROFILE: ${{ vars.AZURE_CERT_PROFILE }}
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
- Run `doctor` on each runner before `build` or `release` so missing SDKs or signing tools fail early.
