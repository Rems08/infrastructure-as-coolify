# Security Policy

## Supported versions

`iac-coolify` is pre-`v1.0.0` beta. Only the latest released version receives security fixes.

| Version | Supported |
|---|---|
| latest `v0.x` | ✅ |
| older | ❌ |

## Reporting a vulnerability

**Please do not open public issues for security vulnerabilities.**

Report privately through GitHub's
[private vulnerability reporting](https://github.com/Rems08/infrastructure-as-coolify/security/advisories/new)
(Security → Advisories → "Report a vulnerability"). We aim to acknowledge reports within
72 hours and to ship a fix or mitigation timeline within 14 days.

When reporting, please include:

- affected version / commit
- a minimal reproduction (config, command, observed vs expected behaviour)
- impact assessment if you have one

## Scope notes

`iac-coolify` mutates remote Coolify infrastructure via a Bearer token and may handle user
secrets. Of particular interest:

- token or secret leakage in logs, `plan`/`apply` output, state cache, or error messages
- bypass of the `secrets.Secret` redaction guarantees
- `validate --strict` failing to detect a plaintext secret committed to YAML

The Coolify Bearer token (`COOLIFY_API_TOKEN`) and SOPS/age private keys are managed by the
operator, never by this tool. Token rotation is performed in the Coolify UI.
