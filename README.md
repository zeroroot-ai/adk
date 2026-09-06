# Gibson Agent Development Kit (ADK)

> **Workflow rules:** see [`zeroroot-ai/.github` → `AGENTS.md`](https://github.com/zeroroot-ai/.github/blob/main/AGENTS.md) for branch / PR / commit / release / rebase rules. SDK bumps arrive as auto-generated PRs from the SDK fan-out workflow.

The agent development kit and `gibson` CLI for [zeroroot.ai](https://zeroroot.ai), the zero-trust agent factory.

The ADK exists so an **AI coding agent** — Claude Code, Cursor, or any
LLM driving your editor — can take "I want a Gibson agent that drains
Kubernetes nodes safely" and turn it into a registered, running
component without you typing the implementation.

It ships one binary, **`gibson`**, that scaffolds a complete component
directory whose entrypoint for the AI is **`AGENTS.md`** — a contract
document grounded in real Gibson SDK source paths. The agent reads
AGENTS.md on first open, picks up the contract (LLM slots, harness API,
proto field 100 = `DiscoveryResult`, manifest schema, lifecycle),
writes the implementation, and uses the same `gibson` verbs to validate
and register the result.

The Gibson runtime contracts (interfaces, manifest types, serving
helpers) live in the [SDK](https://github.com/zeroroot-ai/sdk). The
ADK owns the AI-coder ergonomics around them.

## Install

```sh
go install github.com/zeroroot-ai/adk/gibson/cmd/gibson@latest
gibson --help
```

Requires Go 1.24+. The binary is named `gibson`.

## What an AI-driven session looks like

```sh
# 1. One-time per workspace: pin GIBSON_URL.
gibson init --gibson-url https://api.zeroroot.ai

# 2. Scaffold the component you want the AI to build.
gibson component init prom-scanner --kind tool
cd prom-scanner

# 3. Open Claude Code (or Cursor / your AI editor) here.
claude
```

What you say to Claude:

> Build a Gibson tool that probes a list of HTTPS endpoints for an
> exposed Prometheus `/metrics` route. Read AGENTS.md first for the
> contract. Populate the `Discovery` field on the response with any
> exposed services you find so they land in the GraphRAG.

What Claude does on its own — because every contract it needs is in
the directory:

1. **Reads `AGENTS.md`** — learns the `tool.Tool` interface, the
   field-100 = `gibson.graphrag.v1.DiscoveryResult` contract, the
   proto layout (`api/proto/<name>/v1/<name>.proto` + vendored SDK
   protos), the capability-grant enrollment + runtime credential path.
   SDK source paths are
   bare references (`core/sdk/tool/tool.go`,
   `core/sdk/api/proto/gibson/graphrag/v1/graphrag.proto`, …) so
   Claude can grep them directly.
2. **Reads `prompts/add-method.md` and `prompts/add-discovery.md`** —
   step-by-step recipes for the two changes a tool author actually
   makes. Code examples included.
3. **Edits `api/proto/prom-scanner/v1/prom-scanner.proto`** to add the
   request fields (target list, timeout) and the response fields
   (probe results) — keeping field 100 reserved for `DiscoveryResult`.
4. **Runs `make proto`** — `buf.yaml` / `buf.gen.yaml` and the
   vendored graphrag/taxonomy protos are already in the directory, so
   buf resolves everything without manual config.
5. **Implements `ExecuteProto` in `main.go`** — does the HTTPS probe,
   builds a `DiscoveryResult` with `Hosts` / `Services` entries, fills
   field 100 of the response.
6. **Runs `gibson component validate`** — local schema + proto checks
   confirm field 100 is wired correctly and `main.go` parses.
7. **Runs `make build`** — produces the binary.

You then paste the one-time bootstrap token from the dashboard's
Register wizard (the same capability-grant handshake for every kind):

```sh
gibson component register --token <bootstrap-token>
gibson component run
gibson inspect    # shows the principal's effective FGA grants
```

The `.claude/settings.json` allowlist limits Claude's shell verbs to
`make *`, `gibson *`, `buf *`, `go test ./...`, and `kubectl get *` —
no `kubectl apply`, no `helm install`, no writes against `~/.gibson/`.

## Verb surface

```
gibson init                              # workspace bootstrap
gibson component init <name> --kind …    # scaffold (agent | tool | plugin)
gibson component validate                # local schema + proto checks
gibson component register --token <tok>  # capability-grant enrollment handshake
gibson component run                     # supervise the compiled binary
gibson inspect                           # who am I + my grants
gibson docs schema [component-yaml|plugin-yaml]
                                         # JSON Schema for editors / AI coders

# Mission authoring
gibson mission new [--from-template <name>]  # scaffold a mission CUE file
gibson mission validate <file>               # cue vet + protovalidate
gibson mission render <file>                 # compile to proto-shaped JSON/YAML
gibson mission submit <file.cue>             # CUE→define→run full round trip
                                             # (CreateMissionDefinition + CreateMission)

# Mission drafts (TenantService)
gibson mission draft save --name <n> <file.cue>    # persist a CUE draft
gibson mission draft list                          # list all drafts
gibson mission draft load <draft-id>               # fetch CUE source to stdout
gibson mission draft load --out <file> <draft-id>  # or write to a file
gibson mission draft delete <draft-id>             # delete a draft

# LLM provider management (TenantService)
gibson provider add --name <n> --type <t> [--model <m>] [--cred k=v ...]
gibson provider list
gibson provider delete <name>
gibson provider test --type <t> [--model <m>] [--cred k=v ...]

# Machine identity management (TenantService)
gibson agent enroll --name <n> [--kind agent|tool|plugin]
                                             # prints one-time bootstrap token
gibson agent list [--kind agent|tool|plugin]
gibson agent revoke <principal-id>
```

### Connection flags

Commands that call the daemon (`submit`) accept:

| Flag | Default | Description |
|------|---------|-------------|
| `--daemon` | `localhost:50002` (or `GIBSON_DAEMON_ADDR`) | Daemon gRPC address |
| `--insecure` | false | Plaintext gRPC (development only) |
| `--timeout` | `30s` | RPC deadline |

Commands that call TenantService (`mission draft`, `provider`, `agent`) accept:

| Flag | Default | Description |
|------|---------|-------------|
| `--tenant` | `localhost:50002` (or `GIBSON_TENANT_ADDR`) | Tenant service gRPC address |
| `--tenant-id` | `""` (or `GIBSON_TENANT_ID`) | Tenant ID (required by some RPCs) |
| `--insecure` | false | Plaintext gRPC (development only) |
| `--timeout` | `30s` | RPC deadline |

The CLI **does not** call admin RPCs. Machine identities are provisioned
via `gibson agent enroll` (which calls
`AgentIdentityService.CreateAgentIdentity`); it returns a one-time
bootstrap token. `gibson component register --token <tok>` runs the
capability-grant handshake (ADR-0045) and persists the runtime
credential at `~/.gibson/<kind>/<name>.runtime.json` (+ `.host_key`) for
use by `gibson inspect` and `gibson component run`. The same mechanism
serves every component kind — only the FGA policy differs.

There are no back-compat aliases (no `gibson plugin enroll` etc.).
Pre-spec callers update Makefiles and CI; the migration table is in
`CHANGELOG.md`.

## What you get when you scaffold

`gibson component init my-tool --kind tool` produces:

```
my-tool/
├── component.yaml                    # kind: tool, name, version
├── main.go                           # serve.Tool(&MyTool{})
├── api/proto/my-tool/v1/my-tool.proto   # field 100 = DiscoveryResult
├── proto/vendor/                     # vendored SDK protos (graphrag, taxonomy)
├── buf.yaml, buf.gen.yaml            # buf v2 + STANDARD lint
├── go.mod                            # pinned to SDK release
├── Makefile                          # proto/build/test/register/run/image
├── Dockerfile                        # distroless, non-root
├── README.md                         # 4-command human quickstart
├── AGENTS.md                         # ← the AI agent's contract
├── CLAUDE.md                         # Claude-Code-specific shortcut
├── prompts/                          # add-method, add-discovery,
│                                     # debug-enrollment, deploy-checklist
└── .claude/settings.json             # AI shell allowlist
```

The agent scaffold is the same shape minus the proto files. The plugin
scaffold is Go-first (ADR-0065 R4): a single `handler.go` with typed
Go request/response structs and a hermetic cassette test — no proto, no
buf. The connector scaffold is declarative: one `connector.yaml` plus a
schema-validation smoke test — no Go handler code at all.

## Four component shapes

All four kinds enrol through the **one capability-grant (bootstrap
token) mechanism** (docs ADR-0045) — the role differs by FGA policy, not
by enrollment path.

| Kind   | What it is                                     | Built with              | Enrolled via            |
|--------|------------------------------------------------|-------------------------|-------------------------|
| agent  | LLM-driven gRPC service the daemon dials       | `sdk.NewAgent` + `serve.Agent` | bootstrap-token capability-grant |
| tool   | Stateless gRPC tool, proto in / proto out      | `serve.Tool`            | bootstrap-token capability-grant |
| plugin | Stateful integration, Go-first (manifest + typed Go handlers) | `plugin.Serve` + `plugin.WithHandler` | bootstrap-token capability-grant |
| connector | Declarative MCP integration (no Go — one `connector.yaml`) | catalog manifest (ADR-0065 R6) | enabled from the catalog, no enrollment |

Tools follow a platform-wide rule: **proto field 100 on every tool
response is reserved for `gibson.graphrag.v1.DiscoveryResult`**. The
daemon's DiscoveryProcessor auto-extracts field 100 and writes the
entries into the GraphRAG knowledge graph — no Cypher from the tool.
The tool scaffold encodes this by default.

Plugins use a manifest (`plugin.yaml`, `apiVersion
plugin.gibson.zeroroot.ai/v1`) with declared methods, secrets, runtime
mode (`process | pod | setec`), and lifecycle timeouts.

## Workspace config

`gibson init` writes `./.gibson/workspace.yaml`:

```yaml
gibson_url: https://api.zeroroot.ai
```

Workspace files are non-secret. They MUST NOT contain client_id /
client_secret / bootstrap_token / host_key / password / secret /
token fields — Load() rejects them. They also do not pin a tenant —
tenant context is embedded in the runtime credential the handshake
issues. Runtime credentials live at
`~/.gibson/<kind>/<name>.runtime.json` and `<name>.host_key` (both mode
0600), for every component kind.

## Build + test

```sh
make build               # → bin/gibson
make test                # unit tests (default)
make test-integration    # build-the-scaffold smoke tests; needs network + buf
make update-golden       # regenerate scaffold goldens after intentional changes
```

## Embedded mission CUE schema (DO NOT HAND-EDIT)

The gibson CLI embeds a minimal CUE module at
`gibson/cmd/gibson/cmd/mission/schema/` so that `gibson mission validate`
resolves `import "github.com/zeroroot-ai/sdk/api/proto/gibson/mission/v1"`
without requiring the SDK source tree on the customer's machine.

The file
`gibson/cmd/gibson/cmd/mission/schema/api/proto/gibson/mission/v1/mission_definition_proto_gen.cue`
is GENERATED from the SDK proto at
`opensource/sdk/api/proto/gibson/mission/v1/mission_definition.proto`
via `cue import proto` plus two ADK-specific transforms (package
rename `missionpb`→`v1`, alias-rewrite of the `typespb` import). Do
not edit it by hand — drift between the embedded schema and the
authoritative SDK proto means `gibson mission validate` would
silently accept authoring constructs the daemon then rejects at
submit time.

### Regenerate

When the SDK's `mission_definition.proto` changes (and the SDK is
re-released), refresh the embedded copy:

```sh
# Requires the SDK sibling clone at ../sdk and the cue binary
# (go install cuelang.org/go/cmd/cue@v0.16.1).
make generate
cd gibson && go test ./cmd/gibson/cmd/mission/...   # confirm CUE still loads
git add gibson/cmd/gibson/cmd/mission/schema/ && git commit -m "chore: refresh embedded mission CUE from SDK"
```

The other files in the schema bundle (`cue.mod/module.cue` and the
`typespb` stub at `api/proto/gibson/types/v1/types_stub.cue`) are
hand-maintained — they exist precisely to wrap the auto-generated
file in an offline-friendly module. Edit those by hand when the SDK's
typespb surface or module declaration changes; they are NOT touched
by `make generate`.

### Drift gate

`make check-cue-fresh` (run automatically in CI via
`.github/workflows/ci.yml`) regenerates the embedded CUE and
byte-diffs against the committed copy. PRs that change the SDK
mission proto without refreshing the embedded copy fail this check
with a clear `STALE: run 'make generate' to refresh embedded CUE`
message. The gate has two modes (FULL with SDK sibling, STRUCTURAL
without — see `scripts/check-cue-fresh.sh` for the contract);
neither has a `--skip` flag.

Spec: zeroroot-ai/adk#27.

## Spec

The full design + tasks for this CLI lives at
[`.spec-workflow/specs/adk-developer-workflow/`](../../.spec-workflow/specs/adk-developer-workflow/)
(requirements, design, tasks, implementation logs).

## License and history

Apache License 2.0. See [LICENSE](LICENSE). Copyright Zero Root AI.

Issue and pull request numbers cited in comments and documents dated before 2026-09-05 refer to the tracker before the history reset, archived offline. They do not resolve on GitHub.
