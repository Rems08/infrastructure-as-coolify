# Releasing

The release is **not atomic**: the GitHub composite action consumes the container
image that the release workflow itself publishes, so the action's image pin can
only be bumped *after* the workflow has run. Follow the steps in order.

## Prerequisites

- `main` is green (CI + `make verify`) and contains everything the release ships.
- `CHANGELOG.md` has a `## [X.Y.Z] - YYYY-MM-DD` section cut from `[Unreleased]`,
  and the comparison links at the bottom are updated.
- Every self-reference to the previous version is bumped
  (`docs/how-to/ci-integration.md`, `docs/tutorials/getting-started.md`,
  `docs/how-to/install.md`, `.gitlab/templates/iac-coolify.yml` →
  `IAC_COOLIFY_VERSION`). The **only** reference left on the old version at tag
  time is `.github/action/Dockerfile` (step 4). The Homebrew cask is **not** on
  this list — goreleaser bumps it automatically (see below).

## Sequence

1. **Tag** the release commit on `main` and push:

   ```sh
   git tag vX.Y.Z   # annotated or lightweight — goreleaser only needs the ref
   git push origin vX.Y.Z
   ```

   This triggers `release.yml`: goreleaser builds the cross-platform archives,
   the multi-arch image, signs everything with cosign (keyless) and attaches
   SLSA L3 provenance.

2. **Wait for the workflow** to finish, then verify the artifacts:

   ```sh
   gh run watch --repo Rems08/infrastructure-as-coolify
   docker manifest inspect ghcr.io/rems08/infrastructure-as-coolify:X.Y.Z
   cosign verify ghcr.io/rems08/infrastructure-as-coolify:X.Y.Z \
     --certificate-identity-regexp 'github.com/Rems08/infrastructure-as-coolify' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com
   ```

   The workflow also opens the Homebrew cask PR (head branch
   `cask-update-X.Y.Z`) and enables auto-merge on it; confirm it merged once
   CI passed:

   ```sh
   gh pr list --repo Rems08/infrastructure-as-coolify --head cask-update-X.Y.Z --state all
   ```

3. **If the workflow failed** after publishing *any* artifact: do **not** delete
   and re-push the tag. Tags are immutable once an artifact shipped — fix the
   problem and release `X.Y.Z+1` (or the next `-rc.N`). A tag may only be
   deleted when the run shipped zero artifacts.

4. **Bump the action image pin** now that `ghcr.io/...:X.Y.Z` exists:
   `.github/action/Dockerfile` → `FROM ghcr.io/rems08/infrastructure-as-coolify:X.Y.Z AS bin`.
   Land it on `main` (PR or direct commit).

5. **Re-point the floating `v1` action tag** at the commit from step 4 — the pin
   bump and the `v1` move always travel together, otherwise `uses: ...@v1`
   builds an action wrapping the previous binary:

   ```sh
   git tag -f v1 <sha-of-step-4>
   git push -f origin v1
   ```

6. **Smoke-check the consumers**: the GitLab template downloads the release
   binary by version (`IAC_COOLIFY_VERSION`), the action builds from the GHCR
   image — both must resolve the new version.

## Homebrew cask

This repository doubles as the Homebrew tap: goreleaser renders
`Casks/iac-coolify.rb` from the release archives, pushes it to a branch named
`cask-update-X.Y.Z` using the `CASK_PR_TOKEN` secret, and opens a PR to `main`.
The release workflow then enables **squash auto-merge** on that PR with the
default Actions token, so it merges itself once the required `lint` and `test`
checks pass. Prerelease tags (any tag containing `-`) skip the cask entirely.

- The PR must be opened with a personal access token: a PR created by the
  default `GITHUB_TOKEN` never triggers workflows, so CI would not run and the
  auto-merge would wait forever.
- If a cask PR is still open when the next release is cut (e.g. its CI was
  red), **close it first** — two open cask PRs edit the same file and the
  second cannot merge cleanly.
- If the cask PR was never opened, check the goreleaser log: a cask publish
  failure is logged but does not fail the pipeline, while the auto-merge step
  fails loudly when it finds no PR.

### One-time setup

- `CASK_PR_TOKEN` actions secret: a **fine-grained PAT** scoped to this
  repository only, with **Contents: Read and write** and **Pull requests: Read
  and write**. Store it with
  `gh secret set CASK_PR_TOKEN --repo Rems08/infrastructure-as-coolify`.
- Fine-grained PATs expire (366 days max). An expired token fails the release
  *after* the archives and images shipped; per the immutability rule above,
  rotate the secret and ship `X.Y.Z+1` (or push the cask commit by hand).
- Repository settings: **Allow auto-merge** enabled, and the `main` ruleset
  requires the `lint` and `test` status checks (with an admin bypass so
  maintainers can still push directly to `main`). Without required checks,
  enabling auto-merge would merge the PR instantly, before CI.

## Notes

- `Makefile` derives the dev version from `git describe --match='v*.*.*'`, so
  the floating `v1` tag never pollutes version strings.
- The GitHub Release notes are generated from the tag; the curated narrative
  lives in `CHANGELOG.md`.
