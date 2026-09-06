# AGENTS.md — demo-tool

This directory is a **Gibson tool**. A tool is a stateless, sandboxed
gRPC service that an agent calls to perform a single typed operation.
Tools own their own protobuf schema and run as separate binaries; the
daemon handles dispatch, observability, and **automatic GraphRAG
population** based on what your tool returns.

This file is the contract. If a doc and the SDK source disagree, **the
SDK source wins** — paths below are bare so you can grep them.

## What you implement

A type satisfying `tool.Tool`, defined in `core/sdk/tool/tool.go`:

```go
Name()              string
Version()           string
Description()       string
InputMessageType()  string  // proto fully-qualified message name
OutputMessageType() string
ExecuteProto(ctx, in proto.Message) (proto.Message, error)
Health(ctx)         types.HealthStatus
```

`main.go` ships a stub. Add fields to `api/proto/gibson/tools/demotool/v1/demotool.proto`,
run `make proto`, fill in `ExecuteProto`.

## The platform-wide rule: proto field 100 = DiscoveryResult

**Every tool response message reserves field 100 for
`gibson.graphrag.v1.DiscoveryResult`.** The Gibson daemon's
`DiscoveryProcessor` reflects on every tool response, finds field 100,
and writes the entries (Hosts, Ports, Services, Findings, etc.) into
the GraphRAG (Neo4j) knowledge graph automatically.

You write `Discovery: &graphragpb.DiscoveryResult{...}`. **You write
zero Cypher.** The daemon owns the graph.

This rule is documented in `enterprise/CLAUDE.md`, `structure.md`, and
the platform-level proto guidelines. The SDK source for the
DiscoveryResult type is at
`core/sdk/api/proto/gibson/graphrag/v1/graphrag.proto` (vendored into
this scaffold under `proto/vendor/`).

A worked example for a tool that scans hosts and reports services:

```go
discovery := &graphragpb.DiscoveryResult{}
for _, h := range scanned {
    discovery.Hosts = append(discovery.Hosts, &graphragpb.Host{
        Ip:       h.IP,
        Hostname: ptr(h.Hostname),
    })
    for _, p := range h.Ports {
        discovery.Ports = append(discovery.Ports, &graphragpb.Port{
            Number:   int32(p.Number),
            Protocol: p.Protocol,
            HostId:   h.IP, // edge: Port BELONGS_TO Host
        })
    }
}
return &pb.DemoToolResponse{Discovery: discovery, /* + tool-specific fields */}, nil
```

The cross-references (`HostId`, `PortId`, etc.) become Neo4j edges
automatically. See `prompts/add-discovery.md`.

## Serving

`main.go` already calls:

```go
serve.Tool(&DemoToolTool{})
```

`serve.Tool` is in `core/sdk/serve/serve.go`. It chooses between local
mode (gRPC server on :50051) and platform mode (outbound poll) based
on `PLATFORM_URL` env. In local Kind dev you run as a Deployment +
Service; in production the platform-mode pull pattern is preferred for
horizontal scaling.

## Proto layout

```
api/proto/gibson/tools/demotool/v1/demotool.proto   # your contract
api/gen/                                                     # generated, gitignored
proto/vendor/gibson/graphrag/v1/graphrag.proto   # vendored from SDK
proto/vendor/taxonomy/v1/taxonomy.proto          # vendored from SDK
buf.yaml, buf.gen.yaml                      # buf v2 + STANDARD lint
```

`buf.yaml` declares two paths: `api/proto` (yours) and `proto/vendor`
(read-only deps). `make proto` runs `buf generate` and emits
`api/gen/gibson/tools/demotool/v1/demotool.pb.go`.

## Enrollment + run loop

Every component kind — agent, tool, plugin — enrolls through the **one
capability-grant (CG) mechanism** (docs ADR-0045). The role differs by
policy, not by mechanism.

1. **Mint** — your tenant-admin uses the dashboard's "Register Tool"
   wizard, which returns a single-use **bootstrap token** (24h TTL).
2. **Register** — paste the bootstrap token:
   ```sh
   gibson component register --token <bootstrap-token>
   ```
   Runs the SDK's `capabilitygrant` Bootstrap → Discover → Register
   handshake and persists `~/.gibson/tool/demo-tool.host_key` plus
   `~/.gibson/tool/demo-tool.runtime.json` (both mode 0600). Idempotent.
3. **Run** — `make proto && make build && gibson component run`.
4. **Verify grants** — `gibson inspect`.

## Do not

- Do **not** hand-write Cypher or talk to Neo4j. The daemon owns the
  graph; field 100 is your only contract.
- Do **not** edit files under `proto/vendor/` — they are vendored from
  the SDK and define the contract you depend on.
- Do **not** invent proto field numbers other than 100 for the
  `DiscoveryResult` field on responses.
- Do **not** read or write secrets via env vars. The runtime
  credential is your only credential channel.
- Do **not** commit `~/.gibson/tool/demo-tool.host_key`,
  `.runtime.json`, or the `api/gen/` directory.
- Do **not** add `replace` directives or a workspace-root `go.work`.

## Where to look in the SDK

| Topic                | Path                                       |
|----------------------|--------------------------------------------|
| Tool interface       | `core/sdk/tool/tool.go`                    |
| Tool builder         | `core/sdk/tool/builder.go`                 |
| serve.Tool           | `core/sdk/serve/serve.go`                  |
| DiscoveryResult      | `core/sdk/api/proto/gibson/graphrag/v1/graphrag.proto` |
| Taxonomy enums       | `core/sdk/api/proto/taxonomy/v1/taxonomy.proto`        |
| Runtime enrollment (CG) | `core/sdk/capabilitygrant/`             |

## Naming convention

Per `structure.md`, tools follow `{tool-name}` — e.g. `nmap`, `httpx`,
`nuclei`. The DNS-label regex `^[a-z][a-z0-9-]{0,61}[a-z0-9]$` is
enforced by `gibson component init`.
