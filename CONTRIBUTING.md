# Contributing to infrastructure-as-coolify

Thanks for considering a contribution! This is an OSS Go project (Apache-2.0).

## Ground rules

- **Go 1.22 language target, go1.23.12 toolchain** (pinned in `go.mod` + `Makefile`; see the
  comment there for why). All business logic lives in `internal/`; `cmd/iac-coolify/` only
  parses the CLI.
- **`make verify` must be green before every push.** It runs `gofmt`, `go vet`,
  `golangci-lint` (strict preset), `go test -race -cover`, and `govulncheck`. Never bypass
  hooks with `--no-verify`; fix the root cause instead.
- **No invented Coolify endpoints.** Every API call must exist in the pinned spec
  [`testdata/openapi/coolify-v4.yaml`](testdata/openapi/coolify-v4.yaml). If a needed endpoint
  is missing, open an issue rather than guessing.
- **Secrets are typed.** Sensitive values use the opaque `secrets.Secret` type — never a raw
  `string`. `Secret.Reveal()` is allowlisted by an AST test to `internal/secrets/` and
  `internal/coolify/` only.

## Workflow

1. Fork and branch from `main` (`feat/...`, `fix/...`, `docs/...`).
2. Write table-driven tests with golden files in `testdata/<package>/`.
3. Keep commits as [Conventional Commits](https://www.conventionalcommits.org), one line,
   e.g. `feat(resource): add Database resource`.
4. Run `make verify`, open a PR. CI must pass on linux/darwin/windows × amd64/arm64.

## Code style

We follow [Effective Go](https://go.dev/doc/effective_go), the
[Go Proverbs](https://go-proverbs.github.io/), and the
[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md). In short: clear is
better than clever, errors are values (`%w` wrapping), no panics in business code, structured
logging via `log/slog`. Files over 500 lines are a smell — split them.

## Reporting security issues

See [`SECURITY.md`](SECURITY.md). Do **not** open public issues for vulnerabilities.
