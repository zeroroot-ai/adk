# adk — CLAUDE.md

> **Workflow rules:** see [`zeroroot-ai/.github` → `AGENTS.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md) — canonical for branching / commits / PRs / releases / merging. Conventional Commits MANDATORY. Never push to main. Never force-push.

This file is the per-repo addendum. Workspace-wide concerns live in [`~/Code/zeroroot.ai/CLAUDE.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md); architectural decisions in ``docs/adr/`` (local docs → `adr`).

## TL;DR

The Gibson Agent Development Kit — ships one binary (`gibson`) that scaffolds complete agent/tool/plugin component directories for AI coding agents. The CLI binary lives under `gibson/`; mission templates live under `templates/`. Entry point: `cd gibson && go test ./...` then `make build`.

## Architecture

The repo has two roots: `gibson/` (a standard Go module with the `gibson` binary) and `templates/` (CUE mission templates). The CLI scaffolds component directories with a pre-wired `AGENTS.md` contract, `buf.yaml`, `Makefile`, and `.claude/settings.json` allowlist so an AI agent can implement the component without manual configuration. Key verbs: `gibson component init`, `gibson component validate`, `gibson mission submit/validate/render/draft`.

The CLI embeds a CUE schema for the mission DSL (under `gibson/cmd/gibson/cmd/mission/schema/`) that is generated from the SDK proto via `make generate` / `scripts/regen-cue.sh`. Do not hand-edit the `*_proto_gen.cue` files — they drift from the authoritative SDK proto and break `gibson mission validate` silently.

SDK bumps arrive as auto-generated fan-out PRs from the SDK release workflow. The ADK must stay in sync with the current SDK's mission proto.

## Regen commands

```bash
make build                 # build gibson/bin/gibson
make test                  # unit tests via gibson/go test ./...
make generate              # regen embedded CUE schema from SDK proto (requires sdk sibling at ../sdk)
make check-cue-fresh       # CI drift gate: fails if embedded CUE is stale
make templates-export      # regenerate template.json from template.cue for each template
make update-golden         # regenerate scaffold goldens after intentional scaffold changes
```

## Gotchas

- **Embedded CUE is generated, not hand-maintained.** `gibson/cmd/gibson/cmd/mission/schema/api/proto/gibson/mission/v1/mission_definition_proto_gen.cue` is auto-generated. Editing it by hand will be overwritten by the next `make generate` and will break the `check-cue-fresh` CI gate.
- **`make generate` requires the SDK sibling clone** at `../sdk` (i.e., `opensource/sdk/` relative to the workspace root). In FULL mode it does a real regen; in STRUCTURAL mode (SDK absent) it only validates the sentinel header.
- **Two Makefiles.** The top-level `Makefile` drives templates and the CUE-fresh gate. `gibson/Makefile` drives the Go binary. Run `make build` from repo root (delegates to `gibson/`), or `cd gibson && go build ./cmd/gibson`.
- **Mission template drift gate.** `make templates-check` fails CI if `template.json` is stale relative to `template.cue`. Run `make templates-export` and commit after editing any template.

## Links

- Org-level workflow: [`AGENTS.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md)
- Workspace map: workspace `CLAUDE.md`
- Per-repo ADRs: ``docs/repos/adk/adr/`` (local docs → `repos/adk/adr`)
- Domain glossary: ``docs/glossary.md`` (local docs → `glossary.md`)
- PR checklist: ``docs/agents/pr-checklist.md`` (local docs → `agents/pr-checklist.md`)
