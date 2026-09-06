# Debugging demo-agent enrollment

`gibson component register --token <bootstrap-token>` runs the SDK's
**capability-grant (CG)** handshake — the one enrollment mechanism every
component kind shares (docs ADR-0045). It does three things in order;
the error message names which step failed.

## 1. Bootstrap

The CLI presents the single-use bootstrap token to the daemon's
capability-grant endpoint. Failures look like
`enroll: capabilitygrant bootstrap: ...`.

- **`invalid token`** — the token is single-use and expires after 24h.
  Re-issue from the dashboard's Register Agent wizard.
- **Connectivity** — the token carries the platform URL; verify it is
  reachable with `curl -sS ${GIBSON_URL}/.well-known/openid-configuration`.
- **TLS** — behind a corporate proxy or self-signed CA, the CLI uses
  Go's default cert pool. Add the CA to your system trust store;
  `--insecure` is not supported.

## 2. Discover

The CLI fetches the platform's CG discovery document to learn where to
register. Failures look like `enroll: capabilitygrant discover: ...` and
are almost always connectivity / proxy problems — verify reachability as
above.

## 3. Register

The CLI completes the handshake and persists two files under
`~/.gibson/agent/`, both mode 0600:

- `demo-agent.host_key` — the host signing key used for idempotent
  re-registration without consuming another bootstrap token.
- `demo-agent.runtime.json` — the runtime credential + dial URL.

Failures look like `enroll: capabilitygrant register: ...`:

- **`principal disabled`** — the agent principal was disabled. Tenant
  admin must re-enable it in the dashboard.
- **`host key already exists`** — re-registering with the same install
  is a no-op success. To force a fresh registration, delete
  `~/.gibson/agent/demo-agent.host_key` and re-run.

## After successful register

```sh
gibson inspect
```

Auto-detects the runtime credential, calls `IdentityService.WhoAmI`, and
prints your effective grants. If `components` is empty, your tenant admin
may have minted the principal but not granted it any component access yet
— grants are pure FGA policy, decoupled from enrollment.

## Reading the on-disk shape

```sh
jq . ~/.gibson/agent/demo-agent.runtime.json
```

The runtime credential + dial URL are defined in
`core/sdk/capabilitygrant/`. The SDK refuses to load either file if its
mode permissions are looser than 0600.
