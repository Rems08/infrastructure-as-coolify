GO        ?= go
BIN       := dist/iac-coolify
PKG       := ./cmd/iac-coolify
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin

# Toolchains are decoupled per step because no single Go version satisfies every
# ratchet at once (as of 2026-05):
#   * build/test/vuln run on go1.25.10 — the current patch is the only one govulncheck
#     reports as free of stdlib advisories (go1.22/1.23 are EOL with unpatched CVEs).
#   * lint runs on go1.23.12 — golangci-lint v1.61 is built with go1.23 and cannot read
#     export data produced by go ≥ 1.24, so it must analyse under a go1.23 toolchain.
# Overridable, e.g. `make GOTOOLCHAIN=local test`.
GOTOOLCHAIN     ?= go1.25.10
LINT_GOTOOLCHAIN ?= go1.23.12
export GOTOOLCHAIN

.PHONY: all build test lint fmt fmt-check vet vuln verify clean tools

all: verify

build:
	$(GO) build -o $(BIN) $(PKG)

test:
	$(GO) test ./... -race -coverprofile=cover.out
	@$(GO) tool cover -func=cover.out | tail -1

lint:
	GOTOOLCHAIN=$(LINT_GOTOOLCHAIN) $(GOPATH_BIN)/golangci-lint run

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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH_BIN) v1.61.0
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
