---
name: release
description: Cut a versioned release of the **track** repo — pick the semver bump, roll CHANGELOG.md, tag, and let CI publish the binaries. Use when shipping a new track version (e.g. "release track", "cut v0.2.0", "tag a release"). NOT for ordinary commits/PRs — this is the deploy step.
---

# Release track

Cuts a Semantic-Versioning release. Pushing a `vX.Y.Z` tag triggers
`.github/workflows/release.yml`, which tests, cross-compiles linux + darwin
(amd64/arm64, pure-Go/no CGO), and publishes a GitHub Release whose notes are the
matching `CHANGELOG.md` section. The build version is stamped from the tag via
ldflags into `internal/version` — never hand-edit a version in code.

## Preconditions — verify, don't assume
1. On `main`, clean tree, synced: `git checkout main && git pull --ff-only && git status --short`.
2. CI green on main: `gh run list --workflow=test.yml --branch main --limit 1`.
3. Last release: `git describe --tags --abbrev=0` (none yet ⇒ this is the first).
4. Pick the bump from what changed since the last tag (`git log <lasttag>..HEAD --oneline`):
   **patch** = fixes only · **minor** = new features (backward-compatible) · **major** = breaking. Pre-1.0, breaking changes bump the minor.

## Steps
1. **Roll the changelog.** In `CHANGELOG.md`, move the `## [Unreleased]` items into a
   new `## [X.Y.Z] - YYYY-MM-DD` section (run `date +%F` — never guess the date),
   leave a fresh empty `## [Unreleased]`, and add the link-reference lines at the
   bottom (`[X.Y.Z]: …/releases/tag/vX.Y.Z` and update the `[Unreleased]` compare
   link to `vX.Y.Z...HEAD`). If `Unreleased` is empty, summarize the changes since
   the last tag yourself — keep it human-readable (Added/Changed/Fixed/Removed),
   not a raw commit dump.
2. **Commit + push** to main: `git add CHANGELOG.md && git commit -m "Release vX.Y.Z" && git push origin main` (or via a PR if main is protected — merge it first).
3. **Tag the release commit + push:** `git tag -a vX.Y.Z -m "track vX.Y.Z" && git push origin vX.Y.Z`. The tag must include the leading `v` (required for `go install …@vX.Y.Z`).
4. **Watch CI publish:** find the run (`gh run list --workflow=release.yml --limit 1`), `gh run watch <id> --exit-status`, then confirm `gh release view vX.Y.Z` lists the four binaries + `checksums.txt` and the notes match the changelog section.
5. **Refresh the local server** so `track --version` and the web-UI footer show the new tag: `make install` (or `make build` then restart `track serve`) — the running binary keeps whatever version it was started with, and the browser caches `/api/meta`, so hard-reload (cmd+shift+r) to see it.

## Guardrails
- Never move or re-tag a published tag (Go's module proxy caches them). To fix a bad release, cut the next patch.
- Don't skip the CHANGELOG roll — the release notes are generated from it; an empty section ships empty notes (CI falls back to the auto PR list).
- macOS: binaries **downloaded via a browser** get `com.apple.quarantine` and need `xattr -d com.apple.quarantine <file>`; `go install`/`make install` (local build) avoid this.
