# The binary is copied from the published multi-arch release image rather than
# rebuilt, so the action ships exactly what a release shipped. The version is
# selected by the action git ref (a Docker action cannot interpolate an input
# into its image tag), so each tagged action ref pins a matching image tag.
# A shell base (not the distroless release image) is needed because the
# entrypoint turns the action inputs into CLI flags, and running as root avoids
# the GitHub workspace write-permission issue a non-root image would hit.
FROM ghcr.io/rems08/infrastructure-as-coolify:0.1.4 AS bin

FROM alpine:3.20
COPY --from=bin /iac-coolify /usr/local/bin/iac-coolify

# The entrypoint is written here rather than COPYed so the build does not depend
# on the build-context directory, which is unspecified for a Dockerfile outside
# the action root.
RUN printf '%s\n' \
  '#!/bin/sh' \
  'set -e' \
  'command="$1"' \
  'config_path="$2"' \
  'target="$3"' \
  'envs="$4"' \
  'auto_approve="$5"' \
  'api_token="$6"' \
  'if [ -n "$api_token" ]; then export COOLIFY_API_TOKEN="$api_token"; fi' \
  'set -- "$command" "$config_path"' \
  'if [ -n "$target" ]; then set -- "$@" --target "$target"; fi' \
  'if [ -n "$envs" ]; then' \
  '  for e in $(printf "%s" "$envs" | tr "," " "); do set -- "$@" --env "$e"; done' \
  'fi' \
  'case "$command" in' \
  '  apply|destroy) if [ "$auto_approve" = "true" ]; then set -- "$@" --auto-approve; fi ;;' \
  'esac' \
  'exec iac-coolify "$@"' \
  > /entrypoint.sh && chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
