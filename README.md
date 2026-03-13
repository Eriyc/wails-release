# wailsrel

`wailsrel` is a release and update tool for Wails apps.

## What it is for

Use it when you want one tool to:

- build your app
- publish releases
- create update files
- help the app download new versions

## What it can do

- run your existing Wails build commands
- collect the installers and binaries those builds produce
- generate manifests, indexes, and optional delta patches
- publish releases to GitHub Releases
- generate update files and URLs for your own HTTP server
- support optional frontend bundle updates
- provide Go packages for updater integration in the app

## How it works

1. You add a `wailsrel.yaml` file with your app details, build steps, and release destination.
2. `wailsrel` runs your build steps and finds the output files.
3. It creates the metadata needed for updates.
4. It publishes the release assets and metadata, or prepares update files for your server setup.
5. Your app reads the manifest URL and checks for updates.

## Install

```bash
go install github.com/Eriyc/wailsrel/cmd/wailsrel@latest
```

## Basic flow

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
wailsrel doctor
wailsrel build
wailsrel release
```

## Docs

- [Getting Started](./website/docs/getting-started.md)
- [CLI Commands](./website/docs/commands.md)
- [CI and GitHub Actions](./website/docs/ci.md)
- [Client and Hosting](./website/docs/github-pages.md)
- [Server Contract](./website/docs/server-contract.md)
- [JS Server Example](./website/docs/js-server-example.md)
- [Frontend Runtime](./website/docs/frontend-runtime.md)
