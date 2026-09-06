# Debugging byte-identity enrollment

Plugin enrollment is the **bootstrap-token / capability-grant (CG)**
flow — the one enrollment mechanism every component kind shares (docs
ADR-0045). Agents and tools enroll the same way; a plugin additionally
uploads its `plugin.yaml` manifest at mint time.

## The flow

1. Tenant-admin runs the dashboard's Register Plugin wizard, which
   uploads this directory's `plugin.yaml` to
   `PluginsAdminService.RegisterPlugin`.
2. The daemon validates the manifest, creates a Zitadel
   `plugin_principal`, writes FGA `can_resolve` tuples for every
   declared secret, and returns a single-use bootstrap token (24h
   TTL).
3. Operator runs:
   ```sh
   gibson component register --token <bootstrap-token>
   ```
4. The CLI runs `capabilitygrant.Bootstrap → Discover → Register` and
   persists `~/.gibson/plugin/byte-identity.host_key` plus
   `~/.gibson/plugin/byte-identity.runtime.json` (both mode 0600).

## Step-by-step failure modes

### Bootstrap fails: "invalid token"

The token is single-use and expires after 24h. Re-issue from the
dashboard.

### Discover fails: cannot reach `${GIBSON_URL}`

Networking. Verify with
`curl -sS ${GIBSON_URL}/.well-known/openid-configuration`.

### Register fails: "host key already exists"

The CLI is idempotent: re-registering with the same plugin name
reports success and exits without re-handshaking. If you genuinely
need a fresh registration, delete `~/.gibson/plugin/byte-identity.host_key`
and re-run.

### `gibson component run` exits immediately with non-zero

- **Manifest validation** — a startup-time `Validate` failure prints
  structured per-field errors. Fix `plugin.yaml` and re-run.
- **Method handler missing** — your manifest declares method `X` but
  `handler.go` did not register a handler with `plugin.WithHandler("X", fn)`. SDK
  surfaces this as a method-mismatch error.
- **Secret unavailable at startup** — a `scope: startup` secret
  declared `required: true` but the broker can't resolve it. Check
  the FGA tuple was created (the dashboard does this; if it failed,
  re-register).

### Plugin shows "unreachable" in the dashboard

The plugin's heartbeat to the daemon is missing or the address it
registered is unreachable from the daemon. Three common causes:
- The plugin process crashed (check stdout/stderr).
- Network partition between plugin and daemon.
- Plugin registered itself with `localhost` or a non-routable address
  in non-process runtime modes — verify `plugin_install.address` in
  Redis matches what the daemon can reach.

## Reading registration state

```sh
ls -la ~/.gibson/plugin/byte-identity.*
```

Should list `byte-identity.host_key` and `byte-identity.runtime.json` (both mode
0600). The host key embeds enough to re-authenticate on restart without
consuming another bootstrap token.

```sh
gibson inspect
```

Auto-detects the plugin credentials and prints effective FGA grants.
A newly-registered plugin should show `can_resolve` tuples for every
declared secret.

## Exit code 75 is rotation, not failure

If you see exit code 75, that's the SDK signalling that a
`rotation: restart` secret rotated and the plugin should be restarted.
The CLI's `gibson component run` surfaces 75 verbatim and prints a
clear note. systemd / Kubernetes restart policies pick the plugin
back up automatically.
