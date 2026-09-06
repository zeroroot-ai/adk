# ADK top-level Makefile.
#
# The ADK contains two roots: gibson/ (gibson CLI binary + tooling)
# and templates/ (mission templates dual-published as CUE + JSON +
# MDX). This Makefile drives the templates pipeline and cross-cutting
# checks (check-cue-fresh); use gibson/Makefile for the CLI itself.

.PHONY: templates templates-export templates-vet templates-check \
	ensure-cue ensure-cli bootstrap build test check check-cue-fresh \
	lint lint-new deadcode image generate regen-cue

# Templates ship as triplets: template.cue (authoring source),
# template.json (cue export output, committed for dashboard
# consumption), template.mdx (handwritten description).
TEMPLATES := recon webapp-scan secrets-audit compliance-check scan-fix-verify

# ensure-cue: fail-fast guard for the cue binary.
ensure-cue:
	@command -v cue >/dev/null 2>&1 || { \
		echo "ERROR: cue binary not found on PATH." >&2; \
		echo "  Install with: go install cuelang.org/go/cmd/cue@v0.16.1" >&2; \
		exit 1; \
	}

# templates-export: regenerate template.json for every template
# from its template.cue source via the gibson CLI (schema-aware,
# single code path). Produces proto-shaped JSON (camelCase keys)
# matching the format the daemon and dashboard consume.
# Spec: mission-authoring-cue Requirement 7.
templates-export: ensure-cli
	@for t in $(TEMPLATES); do \
		echo "exporting templates/$$t/template.json"; \
		./gibson/bin/gibson mission render templates/$$t/template.cue > templates/$$t/template.json; \
	done

# ensure-cli: build the gibson CLI binary used for schema-aware vet.
ensure-cli:
	@(cd gibson && go build -o bin/gibson ./cmd/gibson)

# templates-vet: assert each template.cue is structurally valid by
# running it through the gibson CLI validator, which resolves the SDK
# CUE schema via the embedded overlay (single code path, no raw cue).
templates-vet: ensure-cli
	@for t in $(TEMPLATES); do \
		echo "validate templates/$$t/template.cue"; \
		./gibson/bin/gibson mission validate templates/$$t/template.cue || exit 1; \
	done

# templates-check: regenerate template.json files and assert
# `git diff --exit-code`. PRs that change template.cue without
# regenerating template.json fail.
# Spec: mission-authoring-cue Requirement 9 (drift gate).
templates-check: templates-vet templates-export
	@git diff --exit-code templates/ || { \
		echo "ERROR: template.json drifted from template.cue." >&2; \
		echo "  Run \`make templates-export\` and commit the result." >&2; \
		exit 1; \
	}
	@echo "templates-check: ok"

# templates: alias the most common workflow (vet + export).
templates: templates-vet templates-export

# regen-cue: regenerate the embedded mission CUE schema in the gibson
# CLI from the SDK proto. Requires the SDK sibling clone at ../sdk and
# the cue binary on PATH. See scripts/regen-cue.sh for the pipeline
# (cue import proto + ADK-specific package/import normalization).
# Spec: zeroroot-ai/adk#27.
regen-cue:
	@scripts/regen-cue.sh

# check-cue-fresh: drift gate for the ADK-embedded CUE schema.
# FAILS CI when the committed *_proto_gen.cue under
# gibson/cmd/gibson/cmd/mission/schema/ has drifted from the SDK proto.
# Two modes (see scripts/check-cue-fresh.sh for the full contract):
#   FULL       — SDK sibling present: regen + byte-diff.
#   STRUCTURAL — SDK sibling absent : sentinel-header check only.
# Spec: zeroroot-ai/adk#27 (mission-author-experience epic, M3).
check-cue-fresh:
	@scripts/check-cue-fresh.sh

# generate: ALIAS for the maintainer workflow that refreshes the
# embedded mission CUE schema. Failure of check-cue-fresh resolves
# by running `make generate` and committing the result.
generate: regen-cue

# bootstrap: org Makefile contract target. "Just works" one-command dev
# setup — installs the build/lint/dead-code toolchain at the pinned
# versions so `make build|test|check` succeed on a clean checkout. The Go
# toolchain itself is pinned by gibson/go.mod (`go 1.26.4`) + .tool-versions
# and is fetched automatically by `go build` under GOTOOLCHAIN=auto.
# Quality bar: docs/architecture/open-core/RESTRUCTURE-QUALITY-BARS.md §1.
bootstrap:
	@echo "bootstrap: installing pinned dev toolchain"
	go install cuelang.org/go/cmd/cue@v0.16.1
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2
	go install golang.org/x/tools/cmd/deadcode@latest
	@echo "bootstrap: ok"

# build: org Makefile contract target (gibson#171 slice 1.4 /
# zeroroot-ai/.github#87). Builds the gibson CLI binary via the gibson/
# sub-module. Template exports require the built binary; this is the
# prerequisite build step.
build: ensure-cli
	@echo "build: ok"

# test: org Makefile contract target. Delegates to the gibson/ Go module
# so `make test` from the repo root runs the full unit-test suite.
test:
	@(cd gibson && go test ./...)

# lint: full-tree golangci-lint over the gibson/ Go module. No `|| true`
# swallow and no skip-if-absent — a missing binary is a setup error, not a
# pass (run `make bootstrap`). This surfaces the full backlog tracked by
# adk#160; it is NOT yet in the blocking `check` aggregate (the 225-issue
# pre-existing backlog would red-wall main). NEW code is gated by `lint-new`
# in CI. When #160 reaches zero, fold `lint` into `check` and retire
# `lint-new`. Quality bar §3 (lint gates must actually fail).
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "ERROR: golangci-lint not on PATH — run 'make bootstrap'." >&2; \
		exit 1; \
	}
	@(cd gibson && golangci-lint run ./...)

# lint-new: BLOCKING diff-scoped lint gate. Reports only issues introduced
# relative to BASE_REF (default origin/main) so NEW code is held to the full
# linter set while the adk#160 backlog is burned down. This is the gate
# wired into CI; it fails on any lint issue a PR adds.
BASE_REF ?= origin/main
lint-new:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "ERROR: golangci-lint not on PATH — run 'make bootstrap'." >&2; \
		exit 1; \
	}
	@(cd gibson && golangci-lint run --new-from-rev=$(BASE_REF) ./...)

# deadcode: BLOCKING whole-program reachability gate. Any function
# unreachable from the gibson CLI main fails CI. The pre-existing backlog
# (adk#159) is cleared, so there is no allowlist — the gate is fully
# blocking for all dead code. Quality bar §3.
#
# deadcode must be built with the pinned toolchain (go 1.26.4) so it can
# analyze go1.26 source — `go run` pins it via go.mod under GOTOOLCHAIN.
deadcode:
	@command -v deadcode >/dev/null 2>&1 || { \
		echo "ERROR: deadcode not on PATH — run 'make bootstrap'." >&2; \
		exit 1; \
	}
	@out="$$(cd gibson && deadcode ./...)"; \
	if [ -n "$$out" ]; then \
		echo "ERROR: dead (unreachable) code found:" >&2; \
		printf '%s\n' "$$out" >&2; \
		exit 1; \
	fi; \
	echo "deadcode: ok"

# image: org Makefile contract target. The ADK ships a developer CLI
# (`gibson`) distributed as a Go binary, NOT as a container image — there
# is no first-party image to build or mirror-pin here. (The container
# bases under gibson/.../scaffold are customer SCAFFOLD output, pinned to
# the public golang:1.26.4-alpine to track the toolchain, not adk's own
# build.) Target present for uniform-contract parity; intentional no-op.
image:
	@echo "image: adk ships a Go-binary CLI, no first-party container image (no-op)"

# check: top-level aggregate target and org Makefile contract gate. Use
# this as the local pre-push smoke test. Runs the CUE-fresh drift gate plus
# the BLOCKING dead-code gate. Full-tree `lint` is intentionally NOT here
# yet (225-issue backlog, adk#160) — new code is gated by `lint-new` in CI;
# fold `lint` in here once #160 is zero. templates-check is tracked
# separately under adk#28 (pre-existing template.json whitespace drift).
check: check-cue-fresh deadcode
	@echo "check: ok"
