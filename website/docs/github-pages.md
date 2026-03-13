---
sidebar_position: 4
---

# GitHub Pages Deployment

This repository deploys docs with GitHub Actions and GitHub Pages.

## What the workflow does

The workflow in `.github/workflows/docs-pages.yml`:

- installs the `website/` dependencies with `npm ci`
- runs `npm run build`
- uploads the generated `website/build` directory
- deploys that artifact to GitHub Pages

## Repository metadata

The Docusaurus config derives its GitHub Pages settings from `GITHUB_REPOSITORY`:

- repository pages deploy under `/<repo-name>/`
- user or org pages deploy under `/`

That means the same config works whether the repo is named `wails-release` or later renamed, as long as GitHub Actions is building it.

## First-time GitHub setup

1. Push the workflow and `website/` directory to the default branch.
2. In the repository settings, open **Pages**.
3. Set the source to **GitHub Actions**.
4. Run the `Docs Pages` workflow or push another commit to `main`.

After the first successful deployment, GitHub will expose the published site URL in the Pages settings and the workflow environment.
