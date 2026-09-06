# AGENTS.md — byte-identity-secret

This directory is a **Gibson plugin**: a service that integrates an external
system (GitLab, Slack, Jira, HackerOne, …) into Gibson missions. A plugin is
manifest-driven — a single `plugin.yaml` declares identity, methods, secrets,
runtime mode, and lifecycle timeouts — and **Go-first**: the method contracts
come from your Go types, not from a `.proto`.

This file is the contract. If a doc and the SDK source disagree, **the SDK
source wins** — paths below are bare so you can grep them in
`github.com/zeroroot-ai/sdk`.

## What you implement

`handler.go` declares one typed request/response struct pair and one handler
func per method, plus a `main()` that calls `plugin.Serve`:

```go
func echo(ctx context.Context, req EchoRequest) (EchoResponse, error) { ... }

plugin.Serve(
    context.Background(),
    plugin.WithManifest(cmp.Or(os.Getenv("GIBSON_PLUGIN_MANIFEST"), "./plugin.yaml")),
    plugin.WithHandler("Echo", echo),
)
```

`plugin.WithHandler[Req, Resp]` (`plugin/options.go`) derives the method's
JSON-Schema input/output contract from `Req`/`Resp` at registration and sends
it to the daemon. **There is no `.proto`, no `buf`, no generated code.** A type
that cannot be expressed as JSON Schema (e.g. an `any`/interface field) is a
startup error, not a silent pass.

Never put secret values in an error string, panic message, or log line.

## The manifest is the contract

`plugin.yaml` (apiVersion `plugin.gibson.zeroroot.ai/v1`) declares what the
daemon needs to register and dispatch this plugin. Full schema:
`plugin/manifest/manifest.go`. The same `Validate` backs the CLI validator
(`gibson component validate`), the SDK at startup, and the daemon at
registration.

Key spec fields:

- `metadata.name` — DNS-label, regex `^[a-z][a-z0-9-]{0,61}[a-z0-9]$`
- `spec.workload_class` — must be `plugin`
- `spec.runtime` — one of `process | pod | setec` (default `process`)
- `spec.methods[]` — `name` + `description` only (the contract is derived from
  Go, so no `request_proto` / `response_proto`)
- `spec.secrets[]` — broker-resolved credentials (see below)
- `spec.health.startup_timeout` / `liveness_interval` — default 30s / 10s

## The secrets broker — never env vars

**Plugins never read secrets from environment variables or config files.** The
only credential channel is the SDK's secrets broker
(`plugin/secrets/`, `plugin/secret.go`). Declare in `plugin.yaml`:

```yaml
spec:
  secrets:
  - name: cred:api_key      # broker-qualified name
    scope: startup          # "startup" | "per_call"
    rotation: live          # "live"    | "restart"
    required: true
```

Resolve the value from the context the SDK hands your handler
(`secrets.FromContext`). With `rotation: restart` the plugin process exits with
code 75 on rotation; the platform restarts it.

## Testing — hermetic, cassette-driven

`handler_test.go` replays a committed cassette (`testdata/echo.json`) through
the handler and asserts the result. Tests run with a plain `go test ./...` — no
daemon, no network, no build tags (ADR-0065 R7). Add a method → add a cassette
and a sub-test.

## Lifecycle + exit code 75

`plugin/lifecycle/lifecycle.go` defines the state machine
(`Registering → ResolvingSecrets → Starting → Ready → Draining → Stopped`).
Hook transitions with `plugin.WithLifecycle(lifecycle.LifecycleHooks{...})`.
When a `rotation: restart` secret rotates, `plugin.Serve` exits 75 — this is
**not a crash**; `gibson component run` surfaces it verbatim.

## Enrollment + run loop

Every component kind enrolls through the one capability-grant mechanism
(ADR-0045). A plugin additionally uploads its `plugin.yaml` at mint time.

1. **Mint** — the dashboard's Register wizard uploads `plugin.yaml` and returns
   a single-use bootstrap token.
2. **Register** — `gibson component register --token <bootstrap-token>`.
3. **Run** — `make build && gibson component run`.

## Do not

- Do **not** read secrets from env vars — the broker is the only channel.
- Do **not** commit `host_key` or anything under `~/.gibson/`.
- Do **not** include secret values in errors, panics, or logs.
- Do **not** add `request_proto` / `response_proto` back to `plugin.yaml`, or
  reintroduce `.proto`/`buf` — the contract is derived from Go (ADR-0065 R4).
- Do **not** add `replace` directives or a workspace-root `go.work`.
- Do **not** treat exit code 75 as failure.

## Where to look in the SDK (`github.com/zeroroot-ai/sdk`)

| Topic            | Path                             |
|------------------|----------------------------------|
| plugin.Serve     | `plugin/serve.go`                |
| WithHandler      | `plugin/options.go`              |
| Manifest schema  | `plugin/manifest/manifest.go`    |
| Method dispatch  | `plugin/dispatch/`               |
| Secrets broker   | `plugin/secrets/`, `plugin/secret.go` |
| Lifecycle states | `plugin/lifecycle/lifecycle.go`  |
| Health server    | `plugin/health/`                 |
