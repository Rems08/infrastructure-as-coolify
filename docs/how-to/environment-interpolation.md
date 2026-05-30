# Interpolate environment variables

Any visible (Param) string field can reference an environment variable with `${env:VAR}` —
`metadata` names, `image.name`/`image.tag`, `fqdn`, a git `source`, an inline `dockerfile`,
`destination`, `limits`, and plain `env_vars` values. References are resolved at `apply`, just
before the value is pushed; an unset variable is then an error (no silent fallback to `""`).
`validate`, `plan` and `explore` keep the reference unresolved, so they need no variable set.

```yaml
metadata:
  environment: "${env:DEPLOY_ENV}"
spec:
  image:
    tag: "${env:IMAGE_TAG}"
  fqdn: "https://${env:PUBLIC_HOST}"
```

> Param fields are **visible** in plan/apply output once resolved. Never put a secret in a
> Param field — use `value_secret` (see [Manage secrets](secrets.md)) for sensitive values.
