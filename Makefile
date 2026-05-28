GO        ?= go
BIN       := dist/iac-coolify
PKG       := ./cmd/iac-coolify
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin

# Pin the toolchain so local `make verify` matches CI. go1.23.12 is the only line
# that satisfies every ratchet at once: golangci-lint v1.61 (built with go1.23)
# cannot read export data from go ≥ 1.24, and go1.22 is EOL — its stdlib carries
# GO-2025-3750 which govulncheck can never clear. Overridable: `make GOTOOLCHAIN=local`.
GOTOOLCHAIN ?= go1.23.12
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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH_BIN) v1.61.0
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
