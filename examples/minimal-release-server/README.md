# Minimal Release Server

This is the smallest server example in the repo.

It is for native Wails update clients that only need:

- `GET /manifest`
- `GET /delta/manifest`
- `GET /download/:tag/:asset_name`

It does not implement:

- authentication
- protobuf responses
- frontend catalog signing

## Start

```bash
cd examples/minimal-release-server
export GITHUB_REPOSITORY=owner/repo
bun run index.js
```

Set `GITHUB_TOKEN` if the repository is private or you want higher GitHub API limits.

## How it works

1. It reads `manifest.json` and `delta-manifest.json` from the latest GitHub release.
2. It rewrites asset URLs to local `/download/...` URLs.
3. It proxies the real asset bytes from GitHub when the client downloads them.

## Client config

Point the client at:

```text
http://127.0.0.1:8788/manifest
```

## Environment

- `GITHUB_REPOSITORY` required, format `owner/repo`
- `GITHUB_TOKEN` optional
- `HOST` default `127.0.0.1`
- `PORT` default `8788`
- `CACHE_TTL_MS` default `60000`
