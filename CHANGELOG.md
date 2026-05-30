# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added (application mutations & write-back)

- Low-level Coolify client methods for applications: `start`, `stop` and `restart` — the
  `GET /applications/{uuid}/{action}` lifecycle, distinct from the service `POST`
  lifecycle — and environment-variable management: list (`GET .../envs`), bulk update
  (`PATCH .../envs/bulk`, the only create/update path the application API exposes) and
  delete (`DELETE .../envs/{env_uuid}`, where a `404` is a no-op). A secret env value is
  revealed only at the HTTP boundary, never carried by the caller.
- A YAML marshaller (`config.WriteApplication`) that serialises an `Application` back to a
  manifest atomically (temp file then rename, preserving an existing file's mode). A secret
  is written as its `${env:…}`/`${sops:…}` source declaration, never its resolved value, and
  a secret carrying no such declaration is refused before any write. These are internal
  building blocks for an upcoming interactive write-back; no new CLI command is exposed yet.

### Added (explore)

- `iac-coolify explore` (alias `tui`): a terminal browser over a live Coolify instance,
  built on Bubble Tea. It walks the project → environment → resource tree and inspects each
  resource; a service's environment variables are shown as a table, masked by default and
  revealed only on an explicit keypress. It requires an interactive terminal and Coolify
  credentials (no offline mode). Structured logs are surfaced in an in-app pane rather than
  printed to the screen.
- Drift view (`D`): compares the selected application against its desired config (passed as
  an optional `explore <path>` argument), matched by logical name, and shows the per-field
  difference. It is read-only and never prints a secret's value.
- Application lifecycle actions (`R` restart, `S` stop, `U` start): each runs asynchronously
  behind a confirmation prompt that captures every key (so a stray press cannot trigger or
  escape a mutation) and is recorded to the append-only audit log. Editing environment
  variables and writing changes back to YAML are not in the browser yet.
- An application's detail now shows an **ENV VARS (desired)** section read from the matched
  config (matched by `(environment, name)`, never by filename): a plain value is masked until
  `r`, and a secret is shown only by its source declaration (`${env:…}`/`${sops:…}`) — the
  resolved secret value is never read. When no desired Application matches, the section says
  so. Backed by a new `config.LoadApplicationFiles` primitive that keeps each manifest's
  source path (`LoadApplications` delegates to it and strips the paths).

### Added (distribution)

- A composite GitHub Action at the repository root: `uses: Rems08/infrastructure-as-coolify@v1`
  wraps the CLI (`command: plan|apply|destroy|validate`, `path`, `target`, `env`, `api-token`)
  as a Linux Docker action running the published container image. A `action-test` workflow
  self-tests it by validating `examples/minimal` on every push and pull request.
- A reusable GitLab CI template, `.gitlab/templates/iac-coolify.yml`, exposing extendable
  `.iac-coolify-plan`, `.iac-coolify-apply` (default branch and tags) and `.iac-coolify-destroy`
  (manual) jobs. The published image is distroless, so the jobs fetch the static release binary
  and verify its checksum before running it.
- An mdBook documentation site organised by the [Diátaxis](https://diataxis.fr/) framework
  (tutorials, how-to guides, reference, explanation), integrating the generated
  `docs/reference/` as its reference section, deployed to GitHub Pages by a `docs` workflow.
- README CI-integration examples for the Action and the GitLab template, plus a doc-site badge.

## [0.1.0-rc.2] - 2026-05-29

This is the first tagged release: signed, multi-arch binaries and container image with
SLSA build level 3 provenance. It aggregates every change below.

### Added (release pipeline)

- Reproducible release pipeline: `goreleaser` cross-compiles `linux`/`darwin`/`windows`
  for `amd64`/`arm64` (Windows arm64 excluded) into archives with a SHA-256 `checksums.txt`,
  and publishes a multi-arch container image to `ghcr.io/rems08/infrastructure-as-coolify`
  (`linux/amd64`, `linux/arm64`). Each archive is signed with cosign keyless and the build
  emits SLSA build level 3 provenance; see "Verifying release signatures" in the README.
- `make build` embeds the version via `-ldflags`, and `make release-dry` runs the full
  pipeline locally (archives plus local images, no signing or publishing).

### Security (Wave 4.8)

- `APIError` now scrubs any reflected credential from a non-2xx response body before it
  becomes part of an error: a `Bearer <token>` or `CF-Access-Client-Secret: <secret>` echoed
  by the server is replaced with `[REDACTED]`. The token itself is already an opaque
  `Secret`, but a server-reflected copy in the body was a raw string until now.

### Added (Wave 4.8)

- Project hygiene: GitHub issue forms and a PR template, a `examples/` README index with
  worked `minimal/` and `full-stack/` walkthroughs, README status badges (CI, release,
  license, Go version, Go Report Card), a `// Package config` doc comment, and a
  `dependabot.yml` grouping the `golang.org/x/*` and `aws-sdk-go-v2/*` update trees.
- `internal/secrets` test coverage restored to 100% (JSON unmarshal error path and the
  age-key-path edge cases).

### Added (Wave 4b)

- SOPS + age secrets at rest: `value_secret: "${sops:path}"` is decrypted in memory from a
  `secrets.enc.yaml` colocated with the manifest (the path is never user-supplied, so there
  is no traversal surface), navigated by a dotted key such as `databases.staging.password`,
  and stays `[REDACTED]` everywhere. A group- or world-readable age key file is refused
  before any decrypt. Worked example in
  [`examples/secrets-sops/`](examples/secrets-sops/).

### Changed (Wave 4b)

- CI toolchain unified on Go 1.25 and golangci-lint v2, retiring the split that ran the
  linter under an older Go release. The newer toolchain carries the security fixes the SOPS
  dependency tree requires, so `govulncheck` stays clean with zero reachable advisories.

### Added (Wave 4)

- `destroy` command: deletes the declared resources from a live Coolify instance in reverse
  dependency order (applications and services, then environments, then projects). Only
  resources that still exist remotely are deleted, so a repeated destroy is a no-op; a `404`
  on delete is success. Supports `--dry-run`, `--target`, `--auto-approve`, the same exit
  codes (`0`/`1`/`2`) and audit log as `apply`.
- Environment interpolation (L1) extended to every visible (Param) string field —
  `metadata`, `image`, `fqdn`, git `source`, inline `dockerfile`, `destination`, `limits` —
  not just `env_vars` values. An unset reference is an error.
- Audit log enriched with an `actor` field (`IAC_COOLIFY_ACTOR`, else `USER`, else
  `unknown`), alongside the existing secret `sources` and `diff_hash`. The log stays `0600`.
- Secrets documentation: a defence-layer overview and supply modes in the README, plus
  CI templates ([`examples/ci/github-actions.yml`](examples/ci/github-actions.yml),
  [`examples/ci/gitlab-ci.yml`](examples/ci/gitlab-ci.yml)).

### Added (Wave 3.5b)

- `Application` build packs extended from one to six creatable variants: `dockerimage`
  (prebuilt image), `dockerfile` (inline Dockerfile content **or** a git `source`),
  `nixpacks`, `docker-compose`, `static` and `railpack` (public git `source`). `apply`
  routes each to the right Coolify endpoint (`/applications/dockerimage`,
  `/applications/dockerfile`, `/applications/public`).
- `spec.dockerfile` (inline Dockerfile content) and `spec.source`
  (`git_repository` + `git_branch` + `ports_exposes`) fields, with exactly-one-of
  validation per build pack.
- Input-validation ratchets for the new fields: an inline Dockerfile is capped at 1 MB;
  a `git_branch` must match `^[a-zA-Z0-9._/-]{1,255}$` (rejecting option-injection such as
  `--upload-pack=`); a `git_repository` must use an `https://`, `http://` or `git@` scheme.
- The IaC build pack `docker-compose` is translated to Coolify's `dockercompose` spelling.

### Deferred to V0.2

- Applications from a private GitHub App or deploy key (`/applications/private-github-app`,
  `/applications/private-deploy-key`) — they need a separate credential resolver.
- A path-based (rather than inline) Dockerfile.

### Added (Wave 3.5a)

- `Service` resource: a Coolify docker-compose stack sourced from exactly one of
  `docker_compose_path` (a repository compose file) or `type` (a one-click template
  identifier). `validate`, `docs gen` and `apply` support it.
- Path-traversal validation for `docker_compose_path`: absolute paths, control characters
  and any path resolving outside the config tree are rejected before the file is read, so a
  hostile compose path cannot exfiltrate a file outside the repository.
- Coolify Service client methods: create (base64-encoding the compose content),
  update, delete, start/stop/restart, and env-var CRUD plus bulk update.
- `apply` creates services in dependency order (after their project and environment) and
  threads the new service UUID forward; service env vars are applied in one bulk call.
- Audit log records a Service write as `compose_hash` (sha256 of the decoded compose); the
  compose content — which can hold inline secrets — is never written to the log.
- The UUID resolver now also maps services.

### Added (Wave 3)

- `iac-coolify apply`: reconciles the desired state with a live Coolify instance, creating
  resources in dependency order (project → environment → application) via a topological
  sort. Flags: `--auto-approve` (mandatory in a non-interactive session), `--dry-run`
  (offline preview), `--target` (single resource), `--parallelism=1`. Exit codes `0`
  (success), `1` (error), `2` (partial: some resources changed before a failure).
- Reconciliation engine (`internal/apply`): sequential apply with no partial rollback,
  threading newly-created parent UUIDs forward so dependents resolve them. Every write
  carries an `Idempotency-Key` (sha256 of method, path and body) so a retried apply cannot
  create duplicates.
- Append-only audit log (`.iac-coolify/audit.log`, `0600`): one JSON line per applied
  operation recording the resource, op, and the source declarations of any secrets — never
  their resolved values.
- `Project` and `Environment` resources, with `validate` and `docs gen` support.
- Coolify write methods: create/update/delete applications (build-pack-aware endpoint
  selection), create/delete projects and environments. A `404` on delete is a no-op.
- `build_pack` → endpoint mapping for all four build packs. `dockerimage` is creatable from
  the current `Application` schema; the others need source fields not yet modelled and
  return an actionable error.
- The UUID resolver now also maps projects, environments and destination servers.

### Added (Wave 2)

- `iac-coolify plan`: Terraform-style per-field diff between the desired YAML and live
  Coolify state, with `--output=text|json` (auto-detecting TTY/CI), `--detailed-exitcode`
  (`0`/`2`/`1`), and an offline mode (all resources treated as new) when no Coolify
  URL/token is configured.
- Semantic diff engine (`internal/plan`) with Notify-only secret handling: a secret value
  change is announced without ever exposing the value, hash, or any partial.
- Live UUID resolver (`internal/state`): maps logical `(project, environment, kind, name)`
  keys to Coolify UUIDs via the documented `GET /projects`, `/projects/{uuid}/environments`
  and `/applications` endpoints, with an opt-in `--state-cache` JSON file.
- `Database` resource (8 engines: postgresql, mysql, mariadb, mongodb, redis, keydb,
  dragonfly, clickhouse) and a standalone `EnvVar` resource referenced by an Application
  through `env_vars_from`. `validate` and `docs gen` are now multi-resource aware.
- Cloudflare Access support: `CF-Access-Client-Id` / `CF-Access-Client-Secret` headers on
  every request, the secret typed as `Secret` and revealed only at the header boundary.
- OpenAPI checksum verification wired into `plan` boot (refuses a tampered pinned spec,
  tolerates its absence outside the repo).

### Added (Wave 1)

- Repository scaffold: Go module, `internal/` layout, Apache-2.0 license and OSS docs
  (`README`, `CONTRIBUTING`, `CODE_OF_CONDUCT`, `SECURITY`).
- Strict `golangci-lint` preset, `Makefile`, and GitHub Actions CI (lint on go1.23,
  test + cross-build matrix on go1.25) plus a nightly OpenAPI drift watcher.
- Opaque `secrets.Secret` type: redacted across `String`/`GoString`/`MarshalJSON`/
  `MarshalYAML`/`LogValue`, with `${env:VAR}` sourcing, literal values forbidden, and an
  AST ratchet restricting `Reveal()` to `internal/secrets` and `internal/coolify`.
- `${env:VAR}` interpolation for visible (non-secret) values.
- Minimal Coolify v4 API client (`GET /api/v1/applications`) with a Bearer token typed
  as `Secret`, bounded retries, and a pinned-OpenAPI SHA-256 verifier.
- `Application` resource type with `iac:"doc=..."` tags as the single source of truth,
  `ExactlyOneOf{value, value_secret}` validation, and a generated JSON Schema.
- `iac-coolify validate` (strict YAML parsing, semantic validation, and `--strict`
  plaintext-secret detection via regex canaries + entropy) and `iac-coolify version`.
- `iac-coolify docs gen`: Diátaxis reference generated from struct tags, guarded by an
  architecture test that fails when docs drift from the structs.
- State cache type that refuses to marshal if it ever gains a `Secret` field.

[Unreleased]: https://github.com/Rems08/infrastructure-as-coolify/compare/v0.1.0-rc.2...HEAD
[0.1.0-rc.2]: https://github.com/Rems08/infrastructure-as-coolify/releases/tag/v0.1.0-rc.2
