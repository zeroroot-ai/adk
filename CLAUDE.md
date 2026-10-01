# adk — CLAUDE.md

> **Workflow rules:** see [`zeroroot-ai/.github` → `AGENTS.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md) — canonical for branching / commits / PRs / releases / merging. Conventional Commits MANDATORY. Never push to main. Never force-push.

This file is the per-repo addendum. Workspace-wide concerns live in the workspace `CLAUDE.md` and in [`AGENTS.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md).

## TL;DR

The `gibson` CLI and the mission-authoring surface: the binary a person installs to scaffold a component, author a mission in CUE, and submit it. Go module lives under `gibson/`, not the repo root. Start with `make build test check`.

## Architecture

One Go module, `github.com/zeroroot-ai/adk/gibson`, rooted at `gibson/`. The cobra tree is assembled in `gibson/cmd/gibson/cmd/root/root.go`; each command group is its own package beside it: `agent`, `auth`, `component`, `connector`, `docs`, `inspect`, `mission`, `target`, `workspace`.

Every command that reaches the platform dials it over the login session. There is no plaintext path and no unauthenticated path — the daemon resolves the caller's tenant from the bearer token, never from a flag (ADR-0093 decision 4).

Two surfaces are generated and gated, and neither may be hand-edited:

- **The embedded CUE schema** under `gibson/cmd/gibson/cmd/mission/schema/`, so `gibson mission validate` resolves the SDK's mission proto without the SDK source on disk. Stale schema means the CLI accepts authoring the daemon then rejects at submit.
- **The CLI spec**, `gibson docs cli`, a stable sorted JSON document of the whole command tree. The docs-site CLI reference renders from it and a byte-for-byte drift gate compares the committed page.

`templates/<name>/` is a third surface: mission templates dual-published as CUE, JSON and MDX. It is **not** what `gibson mission new` emits — that is `builtinTemplates` in `cmd/mission/new.go`. Changing one does not change the other.

## Commands

```bash
make build            # build the CLI into gibson/bin/
make test             # cd gibson && go test ./...
make check            # check-cue-fresh + readme-matches-cli + deadcode
make templates        # vet and re-export templates/*/template.{cue,json}
make regen-cue        # regenerate the embedded CUE from the SDK (needs a sibling sdk clone + cue)
make lint-new         # golangci-lint on new code only; what CI gates
```

## Gotchas

- **`make lint` is the full backlog, `make lint-new` is the gate.** `lint` surfaces a large pre-existing backlog and is deliberately not in `check`, because turning it on today would red-wall main. CI gates new code with `lint-new`. Run `golangci-lint` locally only under the workspace memory cap, and never while a kind cluster or a race build is running.
- **The README is checked against the binary.** `make readme-matches-cli` fails when the README names a command the cobra tree does not define, or when its Go floor disagrees with `gibson/go.mod`. The floor lives between `<!-- go-floor -->` markers, which is the one literal; do not add a second. It documented `mission draft` and `provider`, two command groups that do not exist, until adk#67.
- **A mission template must name a target.** `mission new` resolves one and writes it in, so the scaffold submits as written. A new template needs exactly one `target_ref:` line or the test suite fails (adk#68).
- **Two template surfaces, one name.** `templates/<name>/template.cue` is documentation a reader fills in; `builtinTemplates` in `new.go` is what the CLI emits. Fix the one the reproduction actually used.
- **`check-cue-fresh` needs a sibling SDK clone and a `cue` binary** to regenerate, which CI has and a bare `go test` does not. Its negative half runs everywhere as a Go test.

## Links

- Org-level workflow: [`AGENTS.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md)
- The SDK this builds on: [`zeroroot-ai/sdk`](https://github.com/zeroroot-ai/sdk)
- Workspace map: workspace `CLAUDE.md`
