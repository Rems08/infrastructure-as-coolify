# Minimal example — 5-minute quickstart

The smallest useful config: a global settings file and a single Application built from a
prebuilt image.

```
minimal/
  coolify.yaml                                   # global: api_url + required_coolify range
  environments/staging/applications/web.yaml     # kind: Application (dockerimage)
```

## 1. Validate (offline)

```sh
iac-coolify validate examples/minimal/
```

This parses the manifests strictly (unknown fields are rejected) and checks every field —
no network, no token.

## 2. Preview against live Coolify

```sh
export COOLIFY_API_TOKEN=...                  # plus --coolify-url or COOLIFY_API_URL
iac-coolify plan examples/minimal/            # runs offline (all-new) if unconfigured
```

`plan` resolves Coolify UUIDs from the logical `metadata` names (never written to YAML),
fetches live state, and prints a per-field diff.

## 3. Edit and re-plan

Change `spec.image.tag` in `web.yaml`, re-run `plan`, and you'll see a single field change
rather than a full create. A second `plan` with no edits reports no changes — applies are
idempotent.

## Next steps

`minimal/` declares only an Application. A real deployment also declares the `Project` and
`Environment` that own it — see [`../full-stack/`](../full-stack/) for the full lifecycle and
[`../full-project/`](../full-project/) for every resource kind.
