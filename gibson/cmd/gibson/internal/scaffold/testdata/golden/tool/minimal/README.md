# demo-tool

A Gibson tool scaffolded by `gibson component init`.

See **[AGENTS.md](./AGENTS.md)** for the Gibson tool contract — including
the platform rule that **proto field 100 on every tool response message
is reserved for `gibson.graphrag.v1.DiscoveryResult`**, which the daemon
auto-extracts into the GraphRAG knowledge graph. What follows is the
five-command quickstart.

## Quickstart

```sh
# 1. Generate Go bindings from api/proto/gibson/tools/demotool/v1/demotool.proto
make proto

# 2. Build
make build

# 3. Register (paste the bootstrap token from the dashboard's Register wizard)
gibson component register --token <bootstrap-token>

# 4. Run (reads the runtime credential from ~/.gibson/tool/)
make run
```

## Container

In a container the runtime credential is supplied as env, not an on-disk
file: `GIBSON_AGENT_KEY` is the base64(JSON) credential (mount it from a
Secret) and `GIBSON_URL` is the daemon dial target.

```sh
make image
docker run --rm \
  -e GIBSON_URL=https://api.zeroroot.ai \
  -e GIBSON_AGENT_KEY="$(jq -c .credential ~/.gibson/tool/demo-tool.runtime.json | base64 -w0)" \
  demo-tool:0.1.0
```

## Proto layout

```
api/proto/gibson/tools/demotool/v1/demotool.proto   # your tool's request/response
api/gen/                                                     # generated Go (gitignored)
proto/vendor/gibson/graphrag/v1/            # vendored DiscoveryResult
proto/vendor/taxonomy/v1/                   # vendored taxonomy enums
buf.yaml, buf.gen.yaml                      # buf v2 configuration
```
