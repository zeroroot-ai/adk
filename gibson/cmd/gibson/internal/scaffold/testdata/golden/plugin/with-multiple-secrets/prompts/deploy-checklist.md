# Deploy checklist for byte-identity-multi

Plugins are stateful and usually have one or two replicas (depending
on whether your underlying integration tolerates concurrent state).
Production plugins live in the helm chart at
`enterprise/deploy/helm/gibson/templates/plugins/`.

## Before deploy

- [ ] `make build` succeeds with the pinned SDK version
- [ ] `gibson component validate` passes (manifest schema clean)
- [ ] All declared secrets exist in the tenant's broker
- [ ] `gibson inspect` shows the plugin principal with the expected
      `can_resolve` grants
- [ ] `make image` produces a tagged image (semver)
- [ ] Image pushed and reachable from the cluster

## Manifest and runtime modes

`spec.runtime` in `plugin.yaml` controls deployment shape:

| Mode      | What it means                           | When to use         |
|-----------|-----------------------------------------|---------------------|
| `process` | Same pod as the daemon (or sidecar)     | Default; dev + small prod |
| `pod`     | Standalone Deployment, gRPC over network| Most production      |
| `setec`   | microVM sandbox per call (Setec)        | Untrusted egress     |

Most plugins use `pod`. Set in the manifest:

```yaml
spec:
  runtime: pod
  policy:
    setec_required: false
```

## Manifest essentials

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: byte-identity-multi
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: plugin
        image: <registry>/byte-identity-multi:0.99.0
        env:
        - name: GIBSON_URL
          value: <daemon URL>
        # Bootstrap token mounted from a Secret seeded by RegisterPlugin.
        # SDK consumes it once on first start, then exclusively uses host_key.
        - name: GIBSON_BOOTSTRAP_TOKEN
          valueFrom:
            secretKeyRef: { name: byte-identity-multi-bootstrap, key: token }
        volumeMounts:
        - name: host-key
          mountPath: /home/nonroot/.gibson/plugin/byte-identity-multi
        ports:
        - containerPort: 8080  # health
        livenessProbe:
          httpGet: { path: /healthz, port: 8080 }
          periodSeconds: 10
      volumes:
      - name: host-key
        emptyDir: {}  # persisted across restarts via PVC in real prod
      terminationGracePeriodSeconds: 60  # plugin.Serve graceful drain
```

## Production discipline

- Production K8s is GitOps-driven (`enterprise/gitops/`). **Do not
  `kubectl apply`** in prod without explicit approval.
- The dev kind cluster is fine for `kubectl apply`.
- Image tags must be immutable.
- `terminationGracePeriodSeconds` ≥ 30 so `OnStop` hooks and the
  drain in `plugin.Serve` complete before SIGKILL.
- The plugin must be allowed to write `~/.gibson/plugin/byte-identity-multi/`
  to persist `host_key`. `runAsUser` matters — use a writeable
  emptyDir or PVC mount.

## Exit code 75 — restart policy

When a `rotation: restart` secret rotates, the plugin exits 75. The
default Kubernetes `restartPolicy: Always` (within a Deployment)
restarts it automatically. Don't add custom logic that conflates 75
with crashes.

## Setec mode

If you set `runtime: setec` and `policy.setec_required: true`, every
invocation runs in a microVM with the manifest's `spec.egress[]` list
as the firewall allowlist. Verify your declared egress targets cover
every external host the plugin contacts.
