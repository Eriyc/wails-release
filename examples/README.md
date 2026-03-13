# Examples

Use these examples to validate the delivery model you want in a consumer app:

- `github-releases-app`: a Wails app that checks a `manifest.json` hosted directly on GitHub Releases and downloads update artifacts from GitHub.
- `authenticated-http-app`: a Wails app that checks a manifest from an HTTP server and downloads update artifacts through an authenticated proxy.
- `authenticated-release-proxy`: a Bun server that proxies GitHub release assets, rewrites manifests back to its own `/download/...` endpoints, and requires a bearer token for artifact downloads.

## GitHub Releases Example

From `examples/github-releases-app`:

```bash
export EXAMPLE_GITHUB_REPOSITORY=owner/repo
export EXAMPLE_CURRENT_VERSION=1.0.0
go run .
```

Optional overrides:

- `EXAMPLE_GITHUB_MANIFEST_URL`
- `EXAMPLE_UPDATE_CHANNEL`
- `EXAMPLE_NATIVE_COMPAT`
- `EXAMPLE_TARGET_PATH`
- `EXAMPLE_TEMP_DIR`

If `EXAMPLE_GITHUB_MANIFEST_URL` is unset, the app derives:

```text
https://github.com/${EXAMPLE_GITHUB_REPOSITORY}/releases/latest/download/manifest.json
```

## Authenticated HTTP Example

Start the Bun proxy first. Then from `examples/authenticated-http-app`:

```bash
export EXAMPLE_PROXY_BASE_URL=http://127.0.0.1:8787
export EXAMPLE_PROXY_TOKEN=change-me
export EXAMPLE_CURRENT_VERSION=1.0.0
go run .
```

Optional overrides:

- `EXAMPLE_PROXY_MANIFEST_URL`
- `EXAMPLE_UPDATE_CHANNEL`
- `EXAMPLE_NATIVE_COMPAT`
- `EXAMPLE_TARGET_PATH`
- `EXAMPLE_TEMP_DIR`

## Bun Proxy

From `examples/authenticated-release-proxy`:

```bash
export GITHUB_REPOSITORY=owner/repo
export GITHUB_TOKEN=ghp_xxx
export PROXY_AUTH_TOKEN=change-me
bun run index.ts
```

Useful optional vars:

- `HOST` default `127.0.0.1`
- `PORT` default `8787`
- `GITHUB_API_BASE_URL` default `https://api.github.com`
- `CACHE_TTL_MS` default `60000`

Routes:

- `GET /manifest.json`
- `GET /delta/manifest.json`
- `GET /download/:tag/:asset_name` with `Authorization: Bearer <token>`
- `GET /healthz`

## Notes

- Both Wails examples compute the current executable checksum automatically when possible so delta updates can be selected when the local binary matches a released artifact.
- Applying updates while running under `go run` targets a temporary Go build cache executable, so packaged builds are the realistic way to exercise replace-and-restart behavior.
