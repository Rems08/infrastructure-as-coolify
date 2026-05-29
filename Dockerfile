# distroless static carries a CA bundle, which the client needs to reach a
# Coolify instance over HTTPS; a scratch base would have none.
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETOS
ARG TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/iac-coolify /iac-coolify
ENTRYPOINT ["/iac-coolify"]
