---
slug: /
sidebar_position: 1
---

# wailsrel

`wailsrel` helps you ship and update Wails apps.

## What it is for

Use it when you want a simple way to:

- build your app
- publish a release
- create update metadata
- let the app download new versions

## What it can do

- run your existing Wails build commands
- collect installers and binaries
- create manifests, indexes, and optional delta patches
- publish to GitHub Releases
- generate update files and URLs for your own HTTP endpoint
- support optional frontend bundle updates

## How it works

1. You describe your app and build steps in `wailsrel.yaml`.
2. `wailsrel` runs those steps and gathers the built files.
3. It creates the files the updater needs.
4. It publishes the assets and metadata, or prepares update files for your server setup.
5. Your app points at the manifest URL and checks for updates.

## Start here

```bash
wailsrel init --name "MyApp" --identifier "com.example.myapp"
wailsrel doctor
wailsrel build
wailsrel release
```

Read [Getting Started](./getting-started.md) for setup, [CLI Commands](./commands.md) for the command list, and [Server Contract](./server-contract.md) if you are serving updates from your own backend.
