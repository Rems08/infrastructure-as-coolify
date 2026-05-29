# Run in CI

Pass `COOLIFY_API_TOKEN` (and any Cloudflare Access service-token headers) as **masked** CI
secrets, and set `IAC_COOLIFY_ACTOR` so the audit log attributes the change. When you use
SOPS, store the age private key as a masked secret and write it to an owner-only file pointed
at by `SOPS_AGE_KEY_FILE`. Run `plan` on pull/merge requests and `apply --auto-approve` on the
protected default branch.

## GitHub Actions

Use the published composite action; the version is selected by the action ref:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Rems08/infrastructure-as-coolify@v1
        with:
          command: plan
          path: coolify/
          env: staging
        # Coolify token, passed from a masked repository secret
        # api-token: ${{ secrets.COOLIFY_API_TOKEN }}
```

`command` is one of `plan|apply|destroy|validate`; `env` accepts several environments
separated by spaces or commas. The action is a Linux Docker action.

## GitLab CI

Include the reusable template and extend the jobs you need:

```yaml
include:
  - remote: "https://raw.githubusercontent.com/Rems08/infrastructure-as-coolify/v0.1.0-rc.2/.gitlab/templates/iac-coolify.yml"

plan:
  extends: .iac-coolify-plan
apply:
  extends: .iac-coolify-apply       # runs on the default branch and tags
destroy:
  extends: .iac-coolify-destroy     # manual only
```

Set `IAC_COOLIFY_PATH` (config directory) and `IAC_COOLIFY_ARGS` (e.g. `--env staging`) as CI
variables, and store `COOLIFY_API_TOKEN` as a masked, protected variable.
