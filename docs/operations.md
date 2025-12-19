# Kafscale Production Operations Guide

This guide explains how to install the Kafscale control plane via Helm, configure S3 and etcd dependencies, and expose the operations console.  It assumes you already have a Kubernetes cluster and the prerequisites listed below.

## Prerequisites

- Kubernetes 1.26+
- Helm 3.12+
- A reachable etcd cluster (the operator uses it for metadata + leader election)
- An S3 bucket per environment plus IAM credentials with permission to list/create objects
- Optional: an ingress controller for the console UI

## Installing with Helm

```bash
helm upgrade --install kafscale deploy/helm/kafscale \
  --namespace kafscale --create-namespace \
  --set operator.etcdEndpoints[0]=http://etcd.kafscale.svc:2379 \
  --set operator.image.tag=v0.1.0 \
  --set console.image.tag=v0.1.0
```

The chart ships the `KafscaleCluster` and `KafscaleTopic` CRDs so the operator can immediately reconcile resources.  Create a Kubernetes secret that contains your S3 access/secret keys, reference it inside a `KafscaleCluster` resource (see `config/samples/`), and the operator will launch broker pods with the right IAM credentials.

### Values to pay attention to

| Value | Purpose |
|-------|---------|
| `operator.replicaCount` | Number of operator replicas (default `2`).  Operators use etcd to elect a leader and stay HA. |
| `operator.leaderKey` | etcd prefix used for the HA lock.  Multiple clusters can coexist by using different prefixes. |
| `console.service.*` | Type/port used to expose the UI.  Combine with `.console.ingress` to publish via an ingress controller. |
| `console.auth.*` | Console login credentials. Set both `console.auth.username` and `console.auth.password` to enable the UI. |
| `imagePullSecrets` | Provide if your container registry (e.g., GHCR) is private. |

## Post-install Steps

1. Apply a `KafscaleCluster` custom resource describing the S3 bucket, etcd endpoints, cache sizes, and credentials secret.
2. Apply any required `KafscaleTopic` resources.  The operator writes the desired metadata to etcd and the brokers begin serving Kafka clients right away.
3. Expose the console UI (optional) by enabling ingress in `values.yaml` or by creating a LoadBalancer service.

## Security & Hardening

- **RBAC** – The Helm chart creates a scoped service account and RBAC role so the operator only touches its CRDs, Secrets, and Deployments inside the release namespace.
- **S3 credentials** – Credentials live in user-managed Kubernetes secrets.  The operator never writes them to etcd.
- **Console auth** – The UI requires `KAFSCALE_UI_USERNAME` and `KAFSCALE_UI_PASSWORD`. There are no defaults; if unset, the login screen shows a warning and the API blocks access. In Helm, set `console.auth.username` and `console.auth.password`, for example:

```bash
helm upgrade --install kafscale deploy/helm/kafscale \
  --set console.auth.username=kafscaleadmin \
  --set console.auth.password='use-a-secret'
```

- **TLS** – Brokers and the console ship HTTPS/TLS flags (`KAFSCALE_BROKER_TLS_*`, `KAFSCALE_CONSOLE_TLS_*`).  Mount certs as secrets via the Helm values and set the env vars to force TLS for client connections.
- **Network policies** – If your cluster enforces policies, allow the operator + brokers to reach etcd and S3 endpoints and lock everything else down.
- **Health / metrics** – Prometheus can scrape `/metrics` on the brokers and operator for early detection of S3 pressure or degraded nodes.  The console also renders the health state for on-call staff.
- **Startup gating** – Broker pods exit immediately if they cannot read metadata or write a probe object to S3 during startup, so Kubernetes restarts them rather than leaving a stuck listener in place.
- **Leader IDs** – Each broker advertises a numeric `NodeID` in etcd. In the single-node demo you’ll always see `Leader=0` in the Console’s topic detail because the only broker has ID `0`. In real clusters those IDs align with the broker addresses the operator published; if you see `Leader=3`, look for the broker with `NodeID 3` in the metadata payload.

## etcd Availability & Storage

Kafscale depends on etcd for metadata + offsets. Treat etcd as a production datastore:

- Run a dedicated etcd cluster (do not share the Kubernetes control-plane etcd).
- Use SSD-backed disks for data and WAL volumes; avoid networked storage when possible.
- Deploy an odd number of members (3 for most clusters, 5 for higher fault tolerance).
- Spread members across zones/racks to survive single-AZ failures.
- Enable compaction/defragmentation and monitor fsync/proposal latency.

### Operator-managed etcd (default path)

If no etcd endpoints are supplied, the operator will provision a 3-node etcd StatefulSet for you. Recommended settings:

- Use an SSD-capable StorageClass for the etcd PVCs (`storageClassName`), with enough IOPS headroom.
- Set a PodDisruptionBudget so only one etcd pod can be evicted at a time.
- Pin etcd pods across zones with topology spread or anti-affinity.
- Enable snapshot backups to a dedicated S3 bucket and retain at least 7 days of snapshots.
- Monitor leader changes, fsync latency, and disk usage; alert on slow or flapping members.

Snapshot job defaults and operator env overrides:

- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_BUCKET` (default: cluster S3 bucket)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_PREFIX` (default: `etcd-snapshots`)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_SCHEDULE` (default: `0 * * * *`)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_ETCDCTL_IMAGE` (default: `quay.io/coreos/etcd:v3.5.12`)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_IMAGE` (default: `amazon/aws-cli:2.15.0`)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_S3_ENDPOINT` (optional, for MinIO or custom S3)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_STALE_AFTER_SEC` (default: 7200)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_CREATE_BUCKET` (optional, set to `1` to auto-create the backup bucket)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_PROTECT_BUCKET` (optional, set to `1` to enable versioning + block public access)
- `KAFSCALE_OPERATOR_ETCD_SNAPSHOT_SKIP_PREFLIGHT` (optional, set to `1` to skip the operator S3 write check)

The operator performs an S3 write preflight before enabling snapshots. If the check fails, the `EtcdSnapshotAccess` condition is set to `False` and reconciliation returns an error until access is restored. Snapshots are uploaded as timestamped files plus a `.sha256` checksum for recovery validation.

## Upgrades & Rollbacks

- Use `helm upgrade --install` with the desired image tags.  The operator drains brokers through the gRPC control plane before restarting pods.
- CRD schema changes follow Kubernetes best practices; run `helm upgrade` to pick them up.
- Rollbacks can be performed with `helm rollback kafscale <REVISION>` which restores the previous deployment and service versions.  Brokers are stateless so the recovery window is short.
