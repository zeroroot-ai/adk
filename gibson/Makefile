BINARY := gibson

.PHONY: build test test-integration update-golden lint vet install fmt check-fmt

## build: compile the gibson CLI binary into ./bin/gibson.
build:
	@mkdir -p bin
	go build -o bin/$(BINARY) ./cmd/gibson

## install: go install the gibson binary into $GOPATH/bin.
install:
	go install ./cmd/gibson

## test: run unit tests (excludes integration tests).
test:
	go test ./...

## test-integration: run integration tests that build scaffolded
## components against the Go module proxy (and buf, when available).
## Requires network access; gated behind the `integration` build tag
## so it doesn't run as part of the default `go test ./...`.
test-integration:
	go test -tags=integration ./...

## update-golden: regenerate scaffold golden files. Run after an
## intentional template change.
update-golden:
	go test -update -run TestRender_PluginGoldenFiles ./cmd/gibson/internal/scaffold

## vet: go vet on the entire CLI tree.
vet:
	go vet ./...

## fmt: gofmt the entire CLI tree in place.
fmt:
	gofmt -w ./cmd

## check-fmt: fail when any committed file has gofmt drift (CI gate).
check-fmt:
	@drift="$$(gofmt -l ./cmd)"; \
	if [ -n "$$drift" ]; then \
		echo "gofmt drift — run 'make fmt' and commit:"; \
		echo "$$drift"; \
		exit 1; \
	fi

## lint: golangci-lint over the CLI tree. A missing binary is a setup
## error, not a silent pass — run `make bootstrap` at the repo root. The
## repo-root `make lint` / `lint-new` targets are the canonical entry
## points (lint-new is the blocking CI gate; see repo-root Makefile +
## adk#160 for the full-tree backlog).
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "ERROR: golangci-lint not on PATH — run 'make bootstrap'." >&2; \
		exit 1; \
	}
	golangci-lint run ./...
