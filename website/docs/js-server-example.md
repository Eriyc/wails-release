---
sidebar_position: 7
---

# JS Server Example

The repo no longer includes a Go gateway. The external server example is the supported pattern.

There are two server examples:

- `examples/minimal-release-server`: the smallest possible server for native update clients
- `examples/authenticated-release-proxy`: a fuller proxy that supports auth and protobuf

Use a JS server to:

- fetch `release-index` and `frontend-index` from GitHub Releases
- filter releases and bundles by auth claims or request parameters
- rewrite asset keys to `/download/:tag/:asset_name`
- proxy GitHub assets without exposing GitHub URLs or tokens
- request a signature from an external frontend catalog signer
- respond with JSON or protobuf depending on `Accept`

Use `examples/minimal-release-server` when you only need native updates from a public or token-backed GitHub release feed. Use `examples/authenticated-release-proxy` when you need auth, protobuf responses, or frontend catalog handling. Both should be treated as application infrastructure, not as part of the `wailsrel` library surface.
