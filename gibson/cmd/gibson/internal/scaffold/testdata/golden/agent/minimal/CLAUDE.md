# CLAUDE.md — demo-agent

Quick guidance for Claude Code (and other AI coders) working in this
directory. **Read [AGENTS.md](./AGENTS.md) for the full Gibson contract.**
This file is the operational shortcut.

## Local dev loop

| Verb                                      | What it does |
|-------------------------------------------|--------------|
| `make build`                              | Compile the binary |
| `make test`                               | Run unit tests |
| `make proto`                              | Regenerate protos *(tool only)* |
| `gibson component validate`               | Local schema checks |
| `gibson component register --token … `    | First-time enrollment (see AGENTS.md) |
| `gibson component run`                    | Run the compiled binary |
| `gibson inspect`                          | Show this principal's effective grants |

## Things not to do

- Do **not** commit `~/.gibson/`, `host_key`, `runtime.json`, or
  the `api/gen/` directory — `.gitignore` excludes them; keep it that
  way.
- Do **not** add `replace` directives to `go.mod` or introduce a
  workspace-root `go.work`. The polyrepo discipline forbids them; pin
  the SDK by tag.
- Do **not** call admin RPCs from this binary. Identity minting is the
  dashboard's job.
- Do **not** hand-edit files under `proto/vendor/` *(tool only)* — they
  are vendored from the SDK and are the contract you depend on, not
  yours to change.

## When in doubt

Read AGENTS.md, then look at the SDK source paths it cites. The SDK is
the contract; this scaffold is just the shape.
