---
sidebar_position: 7
---

# JS Server Example

The repo no longer includes a Go gateway. The external server example is the supported pattern.

Use a JS server to:

- fetch `release-index` and `frontend-index` from GitHub Releases
- filter releases and bundles by auth claims or request parameters
- rewrite asset keys to `/download/:tag/:asset_name`
- proxy GitHub assets without exposing GitHub URLs or tokens
- request a signature from an external frontend catalog signer
- respond with JSON or protobuf depending on `Accept`

The existing Bun example under `examples/authenticated-release-proxy` is the starting point for this pattern. It should be treated as application infrastructure, not as part of the `wailsrel` library surface.
