---
sidebar_position: 6
---

# Server Contract

`wailsrel` no longer ships a built-in server. CI publishes contract artifacts; your server reads them and exposes a client-facing API.

## Published CI artifacts

GitHub Releases should contain both protobuf and JSON variants:

- `manifest.json`
- `manifest.pb`
- `delta-manifest.json`
- `delta-manifest.pb`
- `release-index.json`
- `release-index.pb`
- `frontend-index.json`
- `frontend-index.pb`

The `.pb` and `.json` files are semantically identical.

## Canonical schema source

The source of truth lives under `proto/wailsrel/v1/`.

Generated artifacts in this repo:

- Go types: `gen/go/wailsrel/v1`
- TypeScript types: `gen/ts/wailsrel/v1`
- JSON Schemas: `schema/generated`

## Client-facing endpoints

Your server should support both:

- `Accept: application/json`
- `Accept: application/x-protobuf`

Routes:

- `GET /manifest`
- `GET /delta/manifest`
- `GET /frontend/catalog`
- `GET /download/:tag/:asset_name`

The Go SDK prefers protobuf and falls back to JSON automatically.

## Internal index rules

Published indexes must not contain GitHub download URLs. They should carry asset keys and metadata only:

- tag
- asset key
- checksum
- size
- compat ID
- channel
- published at
- force
- source branch
- commit SHA

Your server is responsible for turning `(tag, asset_key)` into a public download URL.

## Frontend catalog signing

Frontend catalog signing is external to this repo.

The expected flow is:

1. Load `frontend-index.json` or `frontend-index.pb`.
2. Filter bundles by auth, channel, experiment, and compat ID.
3. Rewrite asset keys to your own `/download/:tag/:asset_name` URLs.
4. Send the final unsigned catalog payload to your signer service.
5. Return the signed catalog as JSON or protobuf.
