# Examples

Use these examples to validate the two supported delivery models:

- `github-releases-app`: consume public release artifacts directly from a public `/manifest` endpoint.
- `minimal-release-server`: smallest possible HTTP server for native update clients.
- `authenticated-http-app` plus `authenticated-release-proxy`: consume updates from an HTTP server that serves either JSON or protobuf and proxies asset downloads.

## GitHub Releases Example

From `examples/github-releases-app`:

```bash
export EXAMPLE_GITHUB_REPOSITORY=owner/repo
export EXAMPLE_CURRENT_VERSION=1.0.0
wails3 dev
```

Use `wails3 build` when you want a packaged binary for testing full replace-and-restart behavior.

Optional overrides:

- `EXAMPLE_GITHUB_MANIFEST_URL`
- `EXAMPLE_FRONTEND_CATALOG_URL`
- `EXAMPLE_FRONTEND_CATALOG_PUBLIC_KEY`
- `EXAMPLE_UPDATE_CHANNEL`
- `EXAMPLE_NATIVE_COMPAT`
- `EXAMPLE_TARGET_PATH`
- `EXAMPLE_TEMP_DIR`

If `EXAMPLE_GITHUB_MANIFEST_URL` is unset, the app derives:

```text
https://github.com/owner/repo/releases/latest/download/manifest.json
```

Frontend runtime stays embedded unless you also pin a frontend catalog URL and public key.

## Minimal HTTP Server

From `examples/minimal-release-server`:

```bash
export GITHUB_REPOSITORY=owner/repo
bun run index.js
```

This example is intentionally small:

- native updates only
- JSON responses only
- no auth
- no frontend catalog

Point the client at:

```text
http://127.0.0.1:8788/manifest
```

## HTTP Server Example

Start the Bun server first. Then from `examples/authenticated-http-app`:

```bash
export EXAMPLE_PROXY_BASE_URL=http://127.0.0.1:8787
export EXAMPLE_PROXY_TOKEN=change-me
export EXAMPLE_CURRENT_VERSION=1.0.0
wails3 dev
```

Use `wails3 build` when you want a packaged binary for testing full replace-and-restart behavior.

Optional overrides:

- `EXAMPLE_PROXY_MANIFEST_URL`
- `EXAMPLE_FRONTEND_CATALOG_URL`
- `EXAMPLE_FRONTEND_CATALOG_PUBLIC_KEY`
- `EXAMPLE_UPDATE_CHANNEL`
- `EXAMPLE_NATIVE_COMPAT`
- `EXAMPLE_TARGET_PATH`
- `EXAMPLE_TEMP_DIR`

If `EXAMPLE_PROXY_MANIFEST_URL` is unset, the app derives `EXAMPLE_PROXY_BASE_URL + "/manifest"`.

Frontend runtime stays embedded unless you also pin a frontend catalog public key. When enabled, the app derives `EXAMPLE_PROXY_BASE_URL + "/frontend/catalog"` unless `EXAMPLE_FRONTEND_CATALOG_URL` is set.

## Authenticated Bun HTTP Server

From `examples/authenticated-release-proxy`:

```bash
export GITHUB_REPOSITORY=owner/repo
export PROXY_AUTH_TOKEN=change-me
bun run index.ts
```

Set `GITHUB_TOKEN` when the repo is private or you want higher GitHub API limits. It is optional for public repos.

Useful optional vars:

- `HOST` default `127.0.0.1`
- `PORT` default `8787`
- `GITHUB_API_BASE_URL` default `https://api.github.com`
- `CACHE_TTL_MS` default `60000`

Routes:

- `GET /manifest`
- `GET /delta/manifest`
- `GET /frontend/catalog` returns `501` in this example because frontend signing is external
- `GET /download/:tag/:asset_name` with `Authorization: Bearer <token>`
- `GET /healthz`

Content negotiation examples:

```bash
curl -H 'Accept: application/json' http://127.0.0.1:8787/manifest
curl -H 'Accept: application/x-protobuf' http://127.0.0.1:8787/manifest --output manifest.pb
curl -H 'Accept: application/json' http://127.0.0.1:8787/delta/manifest
curl -H 'Accept: application/x-protobuf' http://127.0.0.1:8787/delta/manifest --output delta-manifest.pb
```

## Notes

- Both Wails examples are full `wails3 init` projects, including the generated `frontend/`, `build/`, and task configuration files.
- Both Wails examples compute the current executable checksum automatically when possible so delta updates can be selected when the local binary matches a released artifact.
- `wails3 dev` is useful for iterating on the UI and updater integration, but packaged builds are the realistic way to exercise replace-and-restart behavior.
