# Why iac-coolify

Coolify is an excellent self-hosted PaaS, but its state lives in a UI and a REST API. As soon
as more than one person — or one environment — is involved, you want the same things Terraform
gives you for cloud resources: a reviewable description of intent, a preview of what will
change, and a repeatable apply. `iac-coolify` brings that model to Coolify natively.

| | `iac-coolify` | `coollabsio/coolify-cli` | `SierraJC/terraform-provider-coolify` |
|---|---|---|---|
| Model | declarative YAML | imperative wrapper | declarative HCL |
| `plan`/`apply`/`destroy` | ✅ `plan` + `apply` + `destroy` | ❌ | ✅ |
| State file | stateless-first | n/a | tfstate required |
| Native to Coolify | ✅ | ✅ | wraps Terraform |

## Stateless-first

There is no state file to store, lock, or drift from reality. `iac-coolify` resolves the live
Coolify UUIDs from your logical `metadata` names at run time and diffs against the actual
remote state. Logical names — never UUIDs — live in your YAML, so a config stays portable
across Coolify instances.

## Idempotent by construction

A second `apply` on an unchanged state is a no-op, and a second `destroy` on already-removed
resources is too (a `404` on delete is success). Every write carries an `Idempotency-Key`, so
a retried apply cannot create duplicates.

## Secrets that cannot leak

Secret values use an opaque type with no visible accessor, so they are `[REDACTED]` in every
log, plan, error and the state cache by construction — not by remembering to redact. See
[Manage secrets](../how-to/secrets.md).
