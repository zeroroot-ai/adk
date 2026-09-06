# Debugging demo-tool enrollment

Tool enrollment is **identical to agent and plugin enrollment** — the
one capability-grant (CG) handshake every component kind shares (docs
ADR-0045) — except the on-disk files live under `~/.gibson/tool/`.

`gibson component register --token <bootstrap-token>` runs CG Bootstrap →
Discover → Register; the error message names which one failed.

## 1. Bootstrap

The CLI presents the single-use bootstrap token. Failure:
`enroll: capabilitygrant bootstrap: ...`.

- `invalid token` — single-use, 24h TTL. Re-issue from the dashboard's
  Register Tool wizard.
- Connectivity — verify with
  `curl -sS ${GIBSON_URL}/.well-known/openid-configuration`.

## 2. Discover

The CLI fetches the CG discovery document. Failure:
`enroll: capabilitygrant discover: ...` — almost always connectivity /
proxy.

## 3. Register

Persists `~/.gibson/tool/demo-tool.host_key` and
`~/.gibson/tool/demo-tool.runtime.json` (both mode 0600). Re-running with
the same install is a no-op success; delete the `.host_key` to force a
fresh registration. `principal disabled` means the tenant admin must
re-enable the principal in the dashboard.

## After register

```sh
gibson inspect
```

Auto-detects the runtime credential, calls `IdentityService.WhoAmI`, and
prints effective component grants. Tools usually need `can_execute` on
whatever components the agent calling them is authorised for; the
dashboard's "Register Tool" flow seeds that automatically when
`component_grants` is supplied.

## Tool-specific gotcha: proto descriptor registration

The first time the tool's binary connects to the daemon, it sends its
`FileDescriptorSet` (the schema for `DemoToolRequest`/`Response`) via
`RegisterComponent`. The daemon caches it. If you change the proto
package or message names, you may need to bump the tool's version
field so the daemon doesn't try to dispatch using the cached old
descriptor.

If `gibson inspect` shows the tool registered but `gibson component
run` reports `unknown message type`, check that the proto FQ name in
your tool's `InputMessageType()` / `OutputMessageType()` matches the
package declaration in `api/proto/gibson/tools/demotool/v1/demotool.proto`.
