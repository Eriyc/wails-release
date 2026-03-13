# HTTP Server Example App

This example is a full Wails 3 project scaffolded with `wails3 init` and then wired to the `wailsrel` updater APIs for the server-backed flow. It is intended to run against the Bun server in [`authenticated-release-proxy`](/Users/eric/Projects/wails-release/examples/authenticated-release-proxy/README.md), which serves `/manifest` and `/delta/manifest` as either JSON or protobuf.

Run it with `wails3 dev` for development or `wails3 build` for a packaged build. See [examples/README.md](/Users/eric/Projects/wails-release/examples/README.md) for the environment variables used by this example.
