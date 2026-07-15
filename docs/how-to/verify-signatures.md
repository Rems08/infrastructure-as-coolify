# Verify release signatures

Every release is signed with [cosign](https://docs.sigstore.dev/cosign/overview/) keyless
(no long-lived key — the signature is bound to this repository's GitHub Actions identity) and
ships [SLSA build level 3](https://slsa.dev/spec/v1.0/levels#build-l3) provenance. Set
`TAG` to the release you downloaded, e.g. `TAG=v0.1.4`.

```sh
# Verify a downloaded archive against its cosign bundle
cosign verify-blob \
  --bundle iac-coolify_${TAG#v}_linux_amd64.tar.gz.bundle \
  --certificate-identity-regexp 'https://github.com/Rems08/infrastructure-as-coolify/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  iac-coolify_${TAG#v}_linux_amd64.tar.gz

# Verify the container image
cosign verify ghcr.io/rems08/infrastructure-as-coolify:${TAG#v} \
  --certificate-identity-regexp 'https://github.com/Rems08/infrastructure-as-coolify/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# Verify the SLSA provenance attached to the release
slsa-verifier verify-artifact \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Rems08/infrastructure-as-coolify \
  --source-tag "$TAG" \
  iac-coolify_${TAG#v}_linux_amd64.tar.gz
```
