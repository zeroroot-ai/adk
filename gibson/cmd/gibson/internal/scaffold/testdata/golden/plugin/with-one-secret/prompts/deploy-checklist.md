# Deploy checklist for byte-identity-secret

Plugins are stateful and usually have one or two replicas (depending
on whether your underlying integration tolerates concurrent state).
Production plugins live in the Gibson umbrella chart under
`helm/gibson-workloads/templates/plugins/`. Install the chart with
`helm install gibson oci://ghcr.io/zeroroot-ai/charts/gibson`. The
chart source is <https://github.com/zeroroot-ai/charts>.

## Before deploy

- [ ] `make build` succeeds with the pinned SDK version
- [ ] `gibson component validate --kind plugin` passes
- [ ] Each secret the plugin resolves exists in the tenant's broker, and a
      tenant admin granted the plugin access to it
- [ ] `gibson inspect` shows the plugin principal with the expected
      `can_resolve` grants
- [ ] `make image` produces a tagged image (semver)
- [ ] Image pushed and reachable from the cluster

## Runtime modes

The `runtime` value of the plugin entry in the chart's `plugins:` map controls
the deployment shape:

| Mode      | What it means                            | When to use     |
|-----------|------------------------------------------|-----------------|
| `pod`     | Standalone Deployment, gRPC over network | Most production |
| `setec`   | microVM sandbox (Setec)                  | Untrusted input |

Most plugins use `pod`.

## Deployment essentials

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: byte-identity-secret
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: plugin
        image: <registry>/byte-identity-secret:1.2.3
        env:
        - name: GIBSON_URL
          value: <daemon URL>
        # Bootstrap token from the dashboard's deploy wizard, mounted from a
        # Secret. SDK consumes it once on first start, then uses host_key.
        - name: GIBSON_BOOTSTRAP_TOKEN
          valueFrom:
            secretKeyRef: { name: byte-identity-secret-bootstrap, key: token }
        volumeMounts:
        - name: host-key
          mountPath: /home/nonroot/.gibson/plugin/byte-identity-secret
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

- Deploy production through your GitOps tree or your Helm release.
  **Do not `kubectl apply`** in prod without explicit approval.
- The dev kind cluster is fine for `kubectl apply`.
- Image tags must be immutable.
- `terminationGracePeriodSeconds` ≥ 30 so `OnStop` hooks and the
  drain in `plugin.Serve` complete before SIGKILL.
- The plugin must be allowed to write `~/.gibson/plugin/byte-identity-secret/`
  to persist `host_key`. `runAsUser` matters — use a writeable
  emptyDir or PVC mount.
