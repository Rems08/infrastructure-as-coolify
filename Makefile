GO        ?= go
BIN       := dist/iac-coolify
PKG       := ./cmd/iac-coolify
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin

# Single toolchain across every step. go1.25.10 is the current patch line govulncheck
# reports free of stdlib advisories; golangci-lint v2 (built with go >= 1.25) analyses
# against it directly. Overridable, e.g. `make GOTOOLCHAIN=local test`.
GOTOOLCHAIN ?= go1.25.10
export GOTOOLCHAIN

.PHONY: all build test lint fmt fmt-check vet vuln verify clean tools

all: verify

build:
	$(GO) build -o $(BIN) $(PKG)

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
