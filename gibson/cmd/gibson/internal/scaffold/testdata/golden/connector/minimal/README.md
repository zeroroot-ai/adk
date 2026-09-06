# demo-connector connector

A Gibson **connector** scaffolded by `gibson component init --kind connector`.

A connector is declarative (ADR-0065 R6): the single `connector.yaml` manifest
IS the integration — no Go, no image build. gibson derives a catalog entry from
it and reconciles a `ConnectorInstance` that the connector-operator maps onto a
ToolHive MCP server (Hosted) or remote proxy (Remote).

See **[AGENTS.md](./AGENTS.md)** for the manifest schema.

## Validate

```sh
go test ./...   # schema-validation smoke test over connector.yaml
```

## Ship it

Move `connector.yaml` into the integrations repo at
`connectors/demo-connector/connector.yaml`. The repo CI runs the same schema check
on every change.
