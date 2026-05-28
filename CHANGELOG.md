# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-05-28

### Added

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

[Unreleased]: https://github.com/Rems08/infrastructure-as-coolify/commits/main
