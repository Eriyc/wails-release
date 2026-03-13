# HTTP Server Example

This Bun server is the companion piece for [`authenticated-http-app`](/Users/eric/Projects/wails-release/examples/authenticated-http-app/README.md). It reads published release artifacts from GitHub Releases, rewrites asset URLs to local `/download/:tag/:asset_name` routes, and serves either JSON or protobuf based on `Accept`.

Use it when you want to test the server-backed delivery path instead of consuming release artifacts directly from a public endpoint. See [examples/README.md](/Users/eric/Projects/wails-release/examples/README.md) for the full setup and environment variables.
