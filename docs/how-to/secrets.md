# Manage secrets

Secrets are an opaque `Secret` type: shown as `[REDACTED]` in every log, plan, apply output,
error, and the state cache — only their origin (`${env:NAME}`) is ever displayed. A literal
secret in YAML is rejected at parse time. The defences layer up:

| Layer | What |
|---|---|
| **L0** Opaque `Secret` type | redaction by construction (the value has no visible accessor) |
| **L1** Env interpolation | `${env:VAR}` in Param fields and `value_secret` |
| **L2** SOPS + age at rest | `${sops:path}` decrypted in memory from a colocated `secrets.enc.yaml` |
| **L4** `validate --strict` | flags a secret-like value mistakenly put in a visible `value` |
| **L5** Redacted logs/plan | guaranteed by L0; no value can reach output |
| **L6** Audit log | append-only `0600` log of who/what, with secret **origins** and a diff hash, never values |

Ways to supply a value:

```yaml
env_vars:
  - name: NODE_ENV
    value: "production"                     # visible literal
  - name: LOG_LEVEL
    value: "${env:LOG_LEVEL}"               # visible, env-resolved
  - name: DATABASE_URL
    value_secret: "${env:DATABASE_URL}"     # secret, env-sourced, REDACTED
  - name: STRIPE_KEY
    value_secret: "${sops:stripe.key}"      # secret, SOPS-decrypted, REDACTED
```

## SOPS + age secrets at rest

A `${sops:path}` reference is read from a `secrets.enc.yaml` **colocated with the manifest**
(the path is never user-supplied, so there is no traversal surface), decrypted in memory, and
shown as `[REDACTED]`. `path` is a dotted key into the decrypted document, e.g.
`databases.staging.password`.

```sh
# 1. Generate an age key and restrict it (iac-coolify refuses a group/other-readable key).
age-keygen -o ~/.config/sops/age/keys.txt && chmod 600 ~/.config/sops/age/keys.txt

# 2. Put your public key (age1...) in your .sops.yaml, then encrypt:
printf 'databases:\n  staging:\n    password: your-secret\n' \
  | sops --encrypt --input-type yaml /dev/stdin > secrets.enc.yaml

# 3. Validate — the SOPS value resolves and stays REDACTED.
iac-coolify validate coolify.yaml
```

`iac-coolify` locates the key via `SOPS_AGE_KEY_FILE` (or the default
`~/.config/sops/age/keys.txt`). **Never commit a private age key** — only the encrypted file
and the public recipient (`.sops.yaml`) are safe to track.

## Viewing secret values

`iac-coolify` never reveals secret values, by design. To inspect one, go to the source — e.g.
`printenv DATABASE_URL`. Secrets stay scoped to the tool that owns them (your shell).
