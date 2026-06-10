# Releasing

The release is **not atomic**: the GitHub composite action consumes the container
image that the release workflow itself publishes, so the action's image pin can
only be bumped *after* the workflow has run. Follow the steps in order.

## Prerequisites

- `main` is green (CI + `make verify`) and contains everything the release ships.
- `CHANGELOG.md` has a `## [X.Y.Z] - YYYY-MM-DD` section cut from `[Unreleased]`,
  and the comparison links at the bottom are updated.
- Every self-reference to the previous version is bumped (`README.md`,
  `docs/how-to/ci-integration.md`, `docs/tutorials/getting-started.md`,
  `.gitlab/templates/iac-coolify.yml` → `IAC_COOLIFY_VERSION`). The **only**
  reference left on the old version at tag time is
  `.github/action/Dockerfile` (step 4).

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

## Notes

- `Makefile` derives the dev version from `git describe --match='v*.*.*'`, so
  the floating `v1` tag never pollutes version strings.
- The GitHub Release notes are generated from the tag; the curated narrative
  lives in `CHANGELOG.md`.
