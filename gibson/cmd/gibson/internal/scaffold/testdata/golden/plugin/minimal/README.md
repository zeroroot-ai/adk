# byte-identity

A Gibson plugin scaffolded by `gibson component init --kind plugin`.

This plugin is **Go-first** (ADR-0065 R4): `handler.go` declares typed request
and response structs and the handler function; the SDK derives each method's
JSON-Schema contract from those Go types at registration. There is no `.proto`
and no generated code.

See **[AGENTS.md](./AGENTS.md)** for the full contract — manifest schema,
lifecycle states, the secrets broker, and exit-code 75. What follows is the
quickstart.

## Quickstart

```sh
# 1. Run the hermetic handler test (cassette replay — no daemon, no network)
go test ./...

# 2. Build
make build

# 3. Register (paste the bootstrap-token from the dashboard)
gibson component register --token <bootstrap-token>

# 4. Run
make run
```

## Add a method

1. Add a typed request/response struct pair and a handler func to `handler.go`.
2. Register it: `plugin.WithHandler("MyMethod", myHandler)`.
3. Declare its name in `plugin.yaml` under `spec.methods`.
4. Record a cassette under `testdata/` and add a sub-test in `handler_test.go`.

## Container (pod runtime mode)

```sh
docker build -t byte-identity:0.1.0 .
```
