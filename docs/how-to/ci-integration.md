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
  - remote: "https://raw.githubusercontent.com/Rems08/infrastructure-as-coolify/v0.1.4/.gitlab/templates/iac-coolify.yml"

plan:
  extends: .iac-coolify-plan
apply:
  extends: .iac-coolify-apply       # runs on the default branch and tags
destroy:
  extends: .iac-coolify-destroy     # manual only
```

Set `IAC_COOLIFY_PATH` (config directory) and `IAC_COOLIFY_ARGS` (e.g. `--env staging`) as CI
variables, and store `COOLIFY_API_TOKEN` as a masked, protected variable.

A plan with pending changes exits 0 — the diff is in the job log and an apply job in a later
stage of the same pipeline can still run. For a gating plan that fails (exit 2) on pending
changes — e.g. in a merge-request pipeline with no apply stage after it — set
`IAC_COOLIFY_PLAN_FLAGS: "--detailed-exitcode"`.

### Supplying `${env:}` values (one secret per application)

`apply` resolves every `${env:KEY}` reference to a real value and pushes it to Coolify, so each
referenced variable must be present in the job environment. Rather than declaring dozens of
individual CI variables, store one **File**-type CI/CD variable per application holding a dotenv
blob, and pass it with `--env-file`:

```
# CI/CD variable RCA_STAGING_ENV (type: File, protected), content:
DJANGO_SETTINGS_MODULE=config.settings.staging
DATABASE_URL=postgres://...
# ... one KEY=VALUE per referenced env var
```

```yaml
apply:
  extends: .iac-coolify-apply
  variables:
    IAC_COOLIFY_ARGS: "--env staging --target my-app --env-file $RCA_STAGING_ENV"
```

A File variable's value is the path GitLab writes the content to, so `--env-file $RCA_STAGING_ENV`
points the binary at it. The file is dotenv format (`KEY=VALUE`, `#` comments, blank lines, an
optional `export ` prefix, optional surrounding quotes; the first `=` splits, so values may
contain `=`). A real environment variable always wins over the file, so tokens injected by CI
(`COOLIFY_API_TOKEN`, `CF_ACCESS_*`) are never shadowed. `plan` accepts `--env-file` too but does
not need it — it compares by reference origin, not resolved value.

### Slow or loaded Coolify instances

Each request times out after 30s by default. When the host is loaded or the topology is large,
set `IAC_COOLIFY_HTTP_TIMEOUT` (a Go duration such as `120s` or `4m`) as a CI variable to raise
it. An unparseable value fails the job loudly rather than being ignored.

### Environment-per-branch convention (main → production, `*-staging` → staging)

A common GitOps split is: merge requests from a `*-staging` branch plan and apply the **staging**
environment, and the default branch applies **production**. The job `rules` and `--env` scope do
this — nothing tool-specific is required:

```yaml
include:
  - remote: "https://raw.githubusercontent.com/Rems08/infrastructure-as-coolify/v0.1.4/.gitlab/templates/iac-coolify.yml"

# Every MR shows the diff for the environment it targets (job is green on no-op).
plan:staging:
  extends: .iac-coolify-plan
  variables: { IAC_COOLIFY_ARGS: "--env staging" }
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event" && $CI_MERGE_REQUEST_SOURCE_BRANCH_NAME =~ /-staging$/'

# A `*-staging` MR pipeline applies staging (staging is the test environment).
# extends brings the apply script; the rules here replace the template's default-branch rule.
apply:staging:
  extends: .iac-coolify-apply
  variables: { IAC_COOLIFY_ARGS: "--env staging" }
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event" && $CI_MERGE_REQUEST_SOURCE_BRANCH_NAME =~ /-staging$/'
      when: manual

# The default branch applies production.
apply:production:
  extends: .iac-coolify-apply
  variables: { IAC_COOLIFY_ARGS: "--env production" }
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
```

Scope further with `--target <name>` to pilot a single application before managing the whole
topology, e.g. `IAC_COOLIFY_ARGS: "--env production --target restaurant-core-api"`.
