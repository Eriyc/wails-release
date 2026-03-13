# Gateway Examples

## Bun / WinterTC-style gateway

`bun-gateway.ts` is a JavaScript reference gateway that implements the same external contract as the Go gateway:

- `GET /manifest.json`
- `GET /delta/manifest.json`
- `GET /download/{tag}/{asset_name}`

It is Bun-first via `Bun.serve()`, but the core logic is written as a WinterTC-style `fetch(request): Response` handler so it can be adapted to other runtimes that expose standard `Request`, `Response`, `fetch`, and WebCrypto APIs.

### Environment

The Bun example uses the same gateway env vars as the Go server:

- `WAILSREL_GATEWAY_ADDR`
- `WAILSREL_GATEWAY_GITHUB_REPOSITORY`
- `WAILSREL_GATEWAY_GITHUB_TOKEN`
- `WAILSREL_GATEWAY_GITHUB_API_BASE_URL`
- `WAILSREL_GATEWAY_JWKS_URL`
- `WAILSREL_GATEWAY_JWT_ISSUER`
- `WAILSREL_GATEWAY_JWT_AUDIENCE`

### Run

```bash
bun run docs/examples/bun-gateway.ts
```

### Notes

- JWT verification is local JWKS validation with `RS256`.
- GitHub asset bytes are proxied server-side; the client never sees the GitHub token.
- Release metadata is cached in memory for 60 seconds.
- The example is intentionally self-contained and dependency-light; if you deploy this for production, add stronger observability, timeouts, and structured error logging.
