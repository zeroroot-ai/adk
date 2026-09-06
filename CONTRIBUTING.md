# Contributing to `adk`

This is the `gibson` CLI and the component scaffold: scaffold a component, build it, validate it, submit a mission. It is the public entry point to the platform.

If anything here is unclear, open an issue rather than guessing — an unclear
contributing guide is a bug in this file.

## Prerequisites

- Go 1.26+
- `make`
The module lives under `gibson/`, not the repository root.

## Build and test

```sh
cd gibson
go build ./...
go test ./...
```

## The merge gate

Scaffolding is golden-tested: `cmd/gibson/internal/scaffold/testdata/golden/`
holds a full generated project per component shape. A change to a template
changes those goldens, and that diff is the review — it shows exactly what every
future `gibson scaffold` will emit.

Every pull request runs it. A red gate is a real signal: **do not** disable a
guard to get a PR through. If a guard is wrong, fix the guard in the same PR
and say why — a guard that needs re-pinning after an unrelated edit is a defect
in the guard.

## Pull requests

- **Conventional Commits in the PR title** — `feat:`, `fix:`, `chore:`,
  `docs:`, `ci:`, `test:`, `refactor:`. The subject must start lowercase;
  `pr-title-lint` enforces both.
- **One root cause per PR.** Two unrelated fixes are two pull requests.
- **Rebase, never merge.** `git fetch origin && git rebase origin/main`
- Releases are automatic via release-please. Never hand-tag, never hand-edit a
  version.

## Reporting a security issue

Do not open a public issue. See [SECURITY.md](SECURITY.md).

## License

Apache-2.0 — see [LICENSE](LICENSE). What you build with this is yours, with no obligation back to us.
