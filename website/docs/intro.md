---
slug: /
sidebar_position: 1
---

# wailsrel

`wailsrel` is a release and update toolkit for Wails v3 applications. It covers the path from versioning and build orchestration through signing, release packaging, and delta updates.

## What is in the repo today

- A Cobra-based CLI with `init`, `status`, `doctor`, `bump`, `build`, `sign`, and `delta`
- YAML config loading with environment expansion, defaults, and validation
- Version discovery and changelog generation from git tags
- Multi-target Wails builds with artifact staging and signing
- Delta patch generation against cached prior release artifacts
- Reference release gateways in both Go and Bun-style JavaScript

## Why this site exists

This Docusaurus site gives the project a versionable docs surface that can be published to GitHub Pages from `main`. It is intentionally separate from the Go toolchain, so docs changes do not affect the CLI build.

## Local development

From the repository root:

```bash
cd website
npm install
npm start
```

The dev server will open a hot-reloading docs site locally. For a production build:

```bash
cd website
npm run build
```
