---
sidebar_position: 3
---

# CLI Commands

The current CLI surface focuses on release automation for Wails applications.

## `init`

Creates or seeds project configuration so `wailsrel` can reason about build, signing, and release inputs.

## `status`

Shows the effective project state, including discovered configuration and release-relevant inputs.

## `doctor`

Checks whether the local environment is ready for the release workflow.

## `bump`

Manages semantic version changes and changelog generation based on git history.

## `build`

Runs the multi-target Wails build pipeline and stages artifacts for release packaging.

## `sign`

Applies platform-specific signing steps to generated artifacts.

## `delta`

Builds delta update packages against previously cached release artifacts.

## Current command pattern

Commands can be executed without installing a binary:

```bash
go run ./cmd/wailsrel <command>
```

Example:

```bash
go run ./cmd/wailsrel doctor
```
