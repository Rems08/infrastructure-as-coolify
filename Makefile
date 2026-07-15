GO        ?= go
BIN       := dist/iac-coolify
PKG       := ./cmd/iac-coolify
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin

# VERSION feeds main.version via ldflags. git describe yields the tag on a
# release commit and the short SHA otherwise; "dev" only when git is absent.
# --match keeps the floating `v1` action tag out of the version string.
VERSION   ?= $(shell git describe --tags --match='v*.*.*' --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)

# Single toolchain across every step. go1.25.12 is the current patch line govulncheck
# reports free of stdlib advisories; golangci-lint v2 (built with go >= 1.25) analyses
# against it directly. Overridable, e.g. `make GOTOOLCHAIN=local test`.
GOTOOLCHAIN ?= go1.25.12
export GOTOOLCHAIN

.PHONY: all build release-dry test lint fmt fmt-check vet vuln verify clean tools

all: verify

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

# release-dry exercises the full goreleaser pipeline locally without signing
# or publishing: archives, checksums and local Docker images land in dist/.
release-dry:
	goreleaser release --snapshot --clean --skip=sign,publish

test:
	$(GO) test ./... -race -coverprofile=cover.out
	@$(GO) tool cover -func=cover.out | tail -1

lint:
	$(GOPATH_BIN)/golangci-lint run

fmt:
	gofmt -w .
	$(GOPATH_BIN)/goimports -w .

# fmt-check fails (non-empty diff) if any file is not gofmt-clean.
fmt-check:
	@gofmt -d . | tee /tmp/iac-fmt-diff
	@test ! -s /tmp/iac-fmt-diff

vet:
	$(GO) vet ./...

vuln:
	$(GOPATH_BIN)/govulncheck ./...

verify: fmt-check vet lint test vuln
	@echo "verify: all green"

clean:
	rm -rf dist cover.out

# tools installs the pinned dev toolchain into $(GOPATH)/bin.
tools:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOPATH_BIN) v2.12.2
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
