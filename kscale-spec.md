# Kafscale: S3-Backed Kafka-Compatible Streaming Platform

A Kubernetes-native, S3-backed message streaming system implementing the Kafka protocol for the 80% of use cases that need durable message delivery without operational complexity.

## Project Philosophy

Most Kafka deployments serve as durable pipes moving data from point A to points B through N. They don't require sub-millisecond latency, exactly-once transactions, or compacted topics. They need: messages in, messages out, consumer tracking, and the confidence that data won't be lost.

Kafscale trades latency for operational simplicity. Brokers are stateless. S3 is the source of truth. Kubernetes handles scaling and failover. The result: a system that scales to zero when idle and requires no dedicated ops expertise.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Kubernetes Cluster                             │
│                                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                          │
│  │   Broker    │  │   Broker    │  │   Broker    │   Stateless pods         │
│  │   Pod 0     │  │   Pod 1     │  │   Pod 2     │   (HPA scaled)           │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                          │
│         │                │                │                                 │
│         └────────────────┼────────────────┘                                 │
│                          │                                                  │
│                          ▼                                                  │
│                   ┌─────────────┐                                           │
│                   │    etcd     │  Metadata store (topic config,            │
│                   │  (3 nodes)  │  consumer offsets, partition assignments) │
│                   └─────────────┘                                           │
│                                                                             │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
                            ┌─────────────┐
                            │     S3      │  Segment storage
                            │   Bucket    │  (source of truth)
                            └─────────────┘
```

### Component Responsibilities

**Broker Pods**: Accept Kafka protocol connections, buffer writes, flush segments to S3, serve reads from S3 (with caching), coordinate consumer groups.

**etcd**: Store topic/partition metadata, consumer group offsets, partition-to-broker assignments. Leverages existing K8s etcd or dedicated cluster.

**S3**: Immutable segment storage. All message data lives here. Brokers have no local persistent state.

---

## Data Model

### Topics and Partitions

```
Topic: "orders"
├── Partition 0
│   ├── segment-00000000000000000000.kfs
│   ├── segment-00000000000000050000.kfs
│   └── segment-00000000000000100000.kfs
├── Partition 1
│   ├── segment-00000000000000000000.kfs
│   └── segment-00000000000000050000.kfs
└── Partition 2
    └── segment-00000000000000000000.kfs
```

### S3 Key Structure

```
s3://{bucket}/{namespace}/{topic}/{partition}/segment-{base_offset}.kfs
s3://{bucket}/{namespace}/{topic}/{partition}/segment-{base_offset}.index
```

Example:
```
s3://kafscale-data/production/orders/0/segment-00000000000000000000.kfs
s3://kafscale-data/production/orders/0/segment-00000000000000000000.index
```

### Segment File Format

Each segment is a self-contained file with messages and metadata:

```
┌────────────────────────────────────────────────────────────────┐
│ Segment Header (32 bytes)                                      │
├────────────────────────────────────────────────────────────────┤
│ Magic Number        │ 4 bytes  │ 0x4B414653 ("KAFS")           │
│ Version             │ 2 bytes  │ Format version (1)            │
│ Flags               │ 2 bytes  │ Compression, etc.             │
│ Base Offset         │ 8 bytes  │ First offset in segment       │
│ Message Count       │ 4 bytes  │ Number of messages            │
│ Created Timestamp   │ 8 bytes  │ Unix millis                   │
│ Reserved            │ 4 bytes  │ Future use                    │
├────────────────────────────────────────────────────────────────┤
│ Message Batch 1                                                │
├────────────────────────────────────────────────────────────────┤
│ Message Batch 2                                                │
├────────────────────────────────────────────────────────────────┤
│ ...                                                            │
├────────────────────────────────────────────────────────────────┤
│ Segment Footer (16 bytes)                                      │
├────────────────────────────────────────────────────────────────┤
│ CRC32               │ 4 bytes  │ Checksum of all batches       │
│ Last Offset         │ 8 bytes  │ Last offset in segment        │
│ Footer Magic        │ 4 bytes  │ 0x454E4421 ("END!")           │
└────────────────────────────────────────────────────────────────┘
```

### Message Batch Format

Messages are grouped into batches for efficiency:

```
┌────────────────────────────────────────────────────────────────┐
│ Batch Header (49 bytes)                                        │
├────────────────────────────────────────────────────────────────┤
│ Base Offset         │ 8 bytes  │ First offset in batch         │
│ Batch Length        │ 4 bytes  │ Total bytes in batch          │
│ Partition Leader    │ 4 bytes  │ Epoch of leader               │
│ Magic               │ 1 byte   │ 2 (Kafka compat)              │
│ CRC32               │ 4 bytes  │ Checksum of batch             │
│ Attributes          │ 2 bytes  │ Compression, timestamp type   │
│ Last Offset Delta   │ 4 bytes  │ Offset of last msg - base     │
│ First Timestamp     │ 8 bytes  │ Timestamp of first message    │
│ Max Timestamp       │ 8 bytes  │ Max timestamp in batch        │
│ Producer ID         │ 8 bytes  │ -1 (no idempotence)           │
│ Producer Epoch      │ 2 bytes  │ -1                            │
│ Base Sequence       │ 4 bytes  │ -1                            │
│ Record Count        │ 4 bytes  │ Number of records in batch    │
├────────────────────────────────────────────────────────────────┤
│ Record 1                                                       │
│ Record 2                                                       │
│ ...                                                            │
└────────────────────────────────────────────────────────────────┘
```

### Individual Record Format

```
┌────────────────────────────────────────────────────────────────┐
│ Length              │ varint   │ Total record size             │
│ Attributes          │ 1 byte   │ Unused (0)                    │
│ Timestamp Delta     │ varint   │ Delta from batch timestamp    │
│ Offset Delta        │ varint   │ Delta from batch base offset  │
│ Key Length          │ varint   │ -1 for null, else byte count  │
│ Key                 │ bytes    │ Message key (optional)        │
│ Value Length        │ varint   │ Message value byte count      │
│ Value               │ bytes    │ Message payload               │
│ Headers Count       │ varint   │ Number of headers             │
│ Headers             │ bytes    │ Key-value header pairs        │
└────────────────────────────────────────────────────────────────┘
```

### Index File Format

Sparse index for offset-to-position lookups:

```
┌────────────────────────────────────────────────────────────────┐
│ Index Header (16 bytes)                                        │
├────────────────────────────────────────────────────────────────┤
│ Magic               │ 4 bytes  │ 0x494458 ("IDX")              │
│ Version             │ 2 bytes  │ 1                             │
│ Entry Count         │ 4 bytes  │ Number of index entries       │
│ Interval            │ 4 bytes  │ Messages between entries      │
│ Reserved            │ 2 bytes  │ Future use                    │
├────────────────────────────────────────────────────────────────┤
│ Entry 1: Offset (8 bytes) + Position (4 bytes)                 │
│ Entry 2: Offset (8 bytes) + Position (4 bytes)                 │
│ ...                                                            │
└────────────────────────────────────────────────────────────────┘
```

---

## Broker Architecture

### Process Structure

```
                    ┌─────────────────────────────────────────────┐
                    │              Broker Process                 │
                    │                                             │
┌─────────┐         │  ┌──────────────────────────────────────┐   │
│ Client  │◄──────► │  │         Network Layer                │   │
│ (TCP)   │         │  │   (Kafka Protocol Handlers)          │   │
└─────────┘         │  └──────────────────┬───────────────────┘   │
                    │                     │                       │
                    │  ┌──────────────────▼───────────────────┐   │
                    │  │         Request Router               │   │
                    │  │   (Metadata, Produce, Fetch, etc.)   │   │
                    │  └─────────────────┬────────────────────┘   │
                    │           ┌────────┴────────┐               │
                    │           ▼                 ▼               │
                    │  ┌─────────────────┐ ┌─────────────────┐    │
                    │  │  Write Path     │ │  Read Path      │    │
                    │  │                 │ │                 │    │
                    │  │ ┌─────────────┐ │ │ ┌─────────────┐ │    │
                    │  │ │Write Buffer │ │ │ │ Segment     │ │    │
                    │  │ │(per partition)│ │ │ Cache       │ │    │
                    │  │ └──────┬──────┘ │ │ └──────┬──────┘ │    │
                    │  │        │        │ │        │        │    │
                    │  │ ┌──────▼──────┐ │ │ ┌──────▼──────┐ │    │
                    │  │ │Segment      │ │ │ │S3 Reader    │ │    │
                    │  │ │Builder      │ │ │ │             │ │    │
                    │  │ └──────┬──────┘ │ │ └──────┬──────┘ │    │
                    │  └────────┼────────┘ └────────┼────────┘    │
                    │           │                   │             │
                    └───────────┼───────────────────┼─────────────┘
                                │                   │
                                ▼                   ▼
                         ┌─────────────┐     ┌─────────────┐
                         │     S3      │     │     S3      │
                         │   (Write)   │     │   (Read)    │
                         └─────────────┘     └─────────────┘
```

### Write Path Detail

```
Producer Request
       │
       ▼
┌──────────────────────────────────────────────────────────────────┐
│ 1. RECEIVE                                                       │
│    - Parse ProduceRequest                                        │
│    - Validate topic exists                                       │
│    - Check this broker owns partition                            │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 2. BUFFER                                                        │
│    - Append to partition's in-memory write buffer                │
│    - Assign offsets (atomic increment from etcd high watermark)  │
│    - Start ack timer if acks=1                                   │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 3. FLUSH DECISION                                                │
│    Check if flush needed:                                        │
│    - Buffer size >= segment_buffer_bytes (default 4MB)           │
│    - Time since last flush >= flush_interval_ms (default 500ms)  │
│    - Explicit flush request                                      │
└───────────────────────────────┬──────────────────────────────────┘
                                │
               ┌────────────────┴────────────────┐
               │ Flush Triggered                 │ No Flush Yet
               ▼                                 ▼
┌─────────────────────────────────┐   ┌─────────────────────────────┐
│ 4. BUILD SEGMENT                │   │ Return to Producer          │
│    - Compress batches (snappy)  │   │ (acks=0: immediate)         │
│    - Build segment header/footer│   │ (acks=1: after buffer)      │
│    - Calculate CRC              │   └─────────────────────────────┘
│    - Build sparse index         │
└───────────────────┬─────────────┘
                    │
                    ▼
┌──────────────────────────────────────────────────────────────────┐
│ 5. S3 UPLOAD                                                     │
│    - Upload segment file (PutObject)                             │
│    - Upload index file (PutObject)                               │
│    - Both uploads must succeed                                   │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 6. COMMIT                                                        │
│    - Update etcd partition state (high watermark, segment list)  │
│    - Clear flushed data from write buffer                        │
│    - Ack waiting producers (acks=all)                            │
└──────────────────────────────────────────────────────────────────┘
```

### Read Path Detail

```
Fetch Request
       │
       ▼
┌──────────────────────────────────────────────────────────────────┐
│ 1. RECEIVE                                                       │
│    - Parse FetchRequest                                          │
│    - Extract: topic, partition, fetch_offset, max_bytes          │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 2. LOCATE SEGMENT                                                │
│    - Load partition metadata from etcd (cached)                  │
│    - Binary search segments to find segment containing offset    │
│    - Calculate S3 key for segment and index                      │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 3. CHECK CACHE                                                   │
│    - Check if segment in local LRU cache                         │
│    - Check if requested range is in read-ahead buffer            │
└───────────────────────────────┬──────────────────────────────────┘
                                │
               ┌────────────────┴────────────────┐
               │ Cache Hit                       │ Cache Miss
               ▼                                 ▼
┌─────────────────────────────────┐   ┌─────────────────────────────┐
│ 4a. SERVE FROM CACHE            │   │ 4b. FETCH FROM S3           │
│     - Read from memory          │   │     - Load index file       │
│     - Skip to step 6            │   │     - Find position in index│
└─────────────────────────────────┘   │     - Range GET from S3     │
                                      │     - Populate cache        │
                                      │     - Trigger read-ahead    │
                                      └──────────────┬──────────────┘
                                                     │
                                ┌────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 5. CHECK UNFLUSHED BUFFER                                        │
│    - If fetch_offset > flushed_offset, also include buffered data│
│    - Merge buffered records with segment data                    │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│ 6. BUILD RESPONSE                                                │
│    - Assemble FetchResponse with record batches                  │
│    - Include high watermark for consumer lag calculation         │
│    - Respect max_bytes limit                                     │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
                          Return to Consumer
```

---

## Kafka Protocol Implementation

### Supported API Keys (v1.0)

Versions reflect what the broker advertises in ApiVersions today.

| API Key | Name | Version | Status | Notes |
|---------|------|---------|--------|-------|
| 0 | Produce | 0-9 | ✅ Full | Core produce path |
| 1 | Fetch | 11-13 | ✅ Full | Core consume path |
| 2 | ListOffsets | 0 | ✅ Full | Required for consumers (v0 only) |
| 3 | Metadata | 0-12 | ✅ Full | Topic/broker discovery |
| 8 | OffsetCommit | 3 | ✅ Full | Consumer group tracking (v3 only) |
| 9 | OffsetFetch | 5 | ✅ Full | Consumer group tracking (v5 only) |
| 10 | FindCoordinator | 3 | ✅ Full | Group coordinator lookup (v3 only) |
| 11 | JoinGroup | 4 | ✅ Full | Consumer group membership (v4 only) |
| 12 | Heartbeat | 4 | ✅ Full | Consumer liveness (v4 only) |
| 13 | LeaveGroup | 4 | ✅ Full | Graceful consumer shutdown (v4 only) |
| 14 | SyncGroup | 4 | ✅ Full | Partition assignment (v4 only) |
| 18 | ApiVersions | 0 | ✅ Full | Client capability negotiation (v0 only) |
| 19 | CreateTopics | 0 | ✅ Full | Topic management (v0 only) |
| 20 | DeleteTopics | 0 | ✅ Full | Topic management (v0 only) |

### Future Releases

| API Key | Name | Target | Notes |
|---------|------|--------|-------|
| 17 | SaslHandshake | v1.1 | SASL/PLAIN and SASL/SCRAM authentication |
| 36 | SaslAuthenticate | v1.1 | Complete auth flow |
| 29 | DescribeAcls | v1.1 | After auth lands |
| 30 | CreateAcls | v1.1 | After auth lands |
| 31 | DeleteAcls | v1.1 | After auth lands |
| 38 | CreateDelegationToken | v2.0 | Enterprise SSO support |
| 39 | RenewDelegationToken | v2.0 | Enterprise SSO support |
| 40 | ExpireDelegationToken | v2.0 | Enterprise SSO support |
| 41 | DescribeDelegationToken | v2.0 | Enterprise SSO support |

- Follow-up work is making sure the broker actually rebalances when the operator changes assignments (e.g., future work on a dedicated controller to rotate leaders rather than the current round-robin snapshot in `BuildClusterMetadata`).

### v1.1 Auth
```
SASL/PLAIN + simple RBAC:
- admin: Create, Delete, Alter, Describe
- producer: Write, Describe  
- consumer: Read, Describe
```
We store credentials in K8s secrets, roles in etcd, auth is disabled per default since most Kafka instances internally run without ACL / RBAC.

### Not Yet Supported (Planned)

| API Key | Name | Notes |
|---------|------|-------|
| 15 | DescribeGroups | Ops debugging - `kafka-consumer-groups.sh --describe` |
| 16 | ListGroups | Ops debugging - enumerate all consumer groups |
| 23 | OffsetForLeaderEpoch | Safe consumer recovery after broker failover |
| 32 | DescribeConfigs | Read topic/broker config |
| 33 | AlterConfigs | Runtime config changes |
| 37 | CreatePartitions | Scale partitions without topic recreation |
| 42 | DeleteGroups | Consumer group cleanup |

### Explicitly Unsupported

| API Key | Name | Reason |
|---------|------|--------|
| 4 | LeaderAndIsr | Internal Kafka protocol, not client-facing |
| 5 | StopReplica | No replication - S3 handles durability |
| 6 | UpdateMetadata | Internal Kafka protocol |
| 7 | ControlledShutdown | Kubernetes handles pod lifecycle |
| 21 | DeleteRecords | S3 lifecycle handles retention |
| 22 | InitProducerId | Transactions not supported (by design) |
| 24 | AddPartitionsToTxn | Transactions not supported |
| 25 | AddOffsetsToTxn | Transactions not supported |
| 26 | EndTxn | Transactions not supported |
| 46 | ListPartitionReassignments | No manual reassignment - S3 is stateless |
| 47 | OffsetDelete | S3 lifecycle handles cleanup |
| 48-49 | DescribeClientQuotas/AlterClientQuotas | Quotas deferred to v2.0 |
| 50-56 | KRaft APIs | Using etcd, not KRaft |
| 57 | UpdateFeatures | Feature flags deferred |
| 65-67 | Transaction APIs (Describe/List/Abort) | Transactions not supported |

### Authentication Roadmap

Kafscale v1.0 ships without authentication, suitable for:
- Internal Kubernetes clusters with network policies
- Development and testing environments
- Proof-of-concept deployments

The authentication roadmap:

| Version | Auth Mechanism | Use Case |
|---------|---------------|----------|
| v1.0 | None | Internal/dev clusters |
| v1.1 | SASL/PLAIN | Username/password (secrets-backed) |
| v1.1 | SASL/SCRAM-SHA-256/512 | Username/password with challenge-response |
| v2.0 | SASL/OAUTHBEARER | Enterprise SSO (Okta, Azure AD, etc.) |
| v2.0 | mTLS | Certificate-based auth |

Until auth lands, Kafscale gracefully handles SASL handshake attempts by returning `UNSUPPORTED_SASL_MECHANISM` (error code 33) rather than dropping the connection, allowing clients to fall back or fail with a clear error message.

---

## Consumer Group Protocol

### State Machine

```
                    ┌─────────┐
           ┌───────►│  Empty  │◄───────┐
           │        └────┬────┘        │
           │             │             │
     All members     First member   All members
        leave          joins          expire
           │             │             │
           │             ▼             │
           │    ┌────────────────┐     │
           │    │ PreparingRe-   │     │
           │    │    balance     │     │
           │    └───────┬────────┘     │
           │            │              │
           │     All members           │
           │     have joined           │
           │            │              │
           │            ▼              │
           │    ┌────────────────┐     │
           │    │ CompletingRe-  │     │
           │    │    balance     │     │
           │    └───────┬────────┘     │
           │            │              │
           │     Leader sends          │
           │     assignments           │
           │            │              │
           │            ▼              │
           │    ┌────────────────┐     │
           └────┤    Stable      ├─────┘
                └───────┬────────┘
                        │
                  Heartbeat timeout
                  or member leaves
                        │
                        ▼
                ┌────────────────┐
                │ PreparingRe-   │
                │    balance     │
                └────────────────┘
```

### JoinGroup Flow

```
Consumer                    Broker                         etcd
   │                          │                              │
   │──JoinGroup Request──────►│                              │
   │                          │                              │
   │                          │◄───Get group state───────────│
   │                          │                              │
   │                          │ (If Empty or Stable)         │
   │                          │───Set PreparingRebalance────►│
   │                          │                              │
   │                          │ (Wait for all members        │
   │                          │  or rebalance timeout)       │
   │                          │                              │
   │◄─JoinGroup Response──────│                              │
   │  (leader gets member     │                              │
   │   list for assignment)   │                              │
   │                          │                              │
```

### SyncGroup Flow

```
Consumer (Leader)           Broker                         etcd
   │                          │                              │
   │──SyncGroup Request──────►│                              │
   │  (with assignments)      │                              │
   │                          │                              │
   │                          │───Store assignments─────────►│
   │                          │                              │
   │                          │───Set Stable────────────────►│
   │                          │                              │
   │◄─SyncGroup Response──────│                              │
   │  (my partition list)     │                              │
   │                          │                              │

Consumer (Follower)         Broker                         etcd
   │                          │                              │
   │──SyncGroup Request──────►│                              │
   │  (empty assignments)     │                              │
   │                          │                              │
   │                          │ (Wait for leader             │
   │                          │  or timeout)                 │
   │                          │                              │
   │◄─SyncGroup Response──────│                              │
   │  (my partition list)     │                              │
   │                          │                              │
```

---

## Caching Strategy

### Multi-Layer Cache Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Broker Process                           │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ L1: Hot Segment Cache (in-process)                         │ │
│  │ - Last N segments per partition                            │ │
│  │ - LRU eviction                                             │ │
│  │ - Typical size: 1-4GB                                      │ │
│  │ - Hit latency: <1ms                                        │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                         Cache Miss                              │
│                              ▼                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ L2: Index Cache (in-process)                               │ │
│  │ - All index files for assigned partitions                  │ │
│  │ - Refreshed on segment roll                                │ │
│  │ - Typical size: 100-500MB                                  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                         Cache Miss                              │
│                              ▼                                  │
└──────────────────────────────┼──────────────────────────────────┘
                               │
                               ▼
                        ┌─────────────┐
                        │     S3      │
                        │  ~50-100ms  │
                        └─────────────┘
```

### Operator etcd Resolution

- `KAFSCALE_OPERATOR_ETCD_ENDPOINTS` (comma-separated list)

The operator resolves etcd endpoints in this order:
1) Use the endpoints declared in `KafscaleCluster.spec.etcd.endpoints`.
2) If unset, fall back to `KAFSCALE_OPERATOR_ETCD_ENDPOINTS`.
3) If still empty, the operator deploys a dedicated 3-node etcd StatefulSet (HA with leader election) and wires the service DNS into the cluster spec.

Disaster recovery: configure an etcd snapshot schedule that uploads backups to a dedicated S3 bucket. The operator should surface the snapshot status and most recent backup timestamp in its status/metrics.

### Etcd Schema Direction (Current)

We currently use a **snapshot-only** schema: the operator publishes a single JSON metadata snapshot to etcd and brokers read from it. We intentionally avoid per-key writes for broker registrations, assignments, or topic metadata until the ops surface requires it. This keeps the control plane simple and avoids partial updates that could drift out of sync.

Future release: consider moving to a per-key schema later (e.g., when we need richer ops APIs or finer-grained updates).

### Environment Variable Overrides

See `docs/development.md` for the current environment variable catalog and test targets. The broker and operator read `KAFSCALE_*` settings at startup and log their effective configuration.

### UI Authentication

- `KAFSCALE_UI_USERNAME`
- `KAFSCALE_UI_PASSWORD`

The React SPA always renders a login screen inspired by MinIO’s aesthetic. Credentials are sourced exclusively from these environment variables—there are no baked-in defaults. If either variable is unset, the login form instead shows an inline warning that UI access is disabled until both values are configured, preventing accidental deployments that rely on hard-coded passwords. The API/auth middleware reads the same variables and rejects every login attempt until they are provided, keeping the UX and backend behavior in sync.

Status: Done (console login + env-gated auth enforced).

---

## Error Handling

The broker maps internal failures to Kafka error codes and returns them in standard response payloads. Client retries should follow Kafka client defaults; for S3-specific failures see the gating behavior below.

### S3 Health Gating

When the broker detects degraded or unavailable S3 health, it rejects Produce and Fetch requests with backpressure error codes (no buffering). Producers receive a failure response immediately so clients can retry; consumers receive fetch errors until S3 recovers. This preserves the “S3 is source of truth” durability contract.

---

## Development Milestones

### Milestone 1: Core Protocol

- [x] Kafka protocol frame parsing
- [x] ApiVersions request/response
- [x] Metadata request/response
- [x] Basic connection handling
- [x] Unit tests for protocol parsing

### Milestone 2: Storage Layer

- [x] Segment file format implementation
- [x] Index file format implementation
- [x] S3 upload/download
- [x] Write buffer with flush logic
- [x] LRU segment cache

### Milestone 3: Produce Path

- [x] ProduceRequest handling
- [x] Offset assignment
- [x] Batch buffering
- [x] S3 segment flush
- [x] etcd metadata updates

### Milestone 4: Fetch Path

- [x] FetchRequest handling
- [x] Segment location logic
- [x] S3 range reads
- [x] Cache integration
- [x] Read-ahead implementation

### Milestone 5: Consumer Groups

- [x] FindCoordinator
- [x] JoinGroup / SyncGroup / Heartbeat
- [x] Partition assignment (range strategy)
- [x] OffsetCommit / OffsetFetch
- [x] Consumer group state machine

### Milestone 6: Admin Operations

- [x] CreateTopics (v0 only)
- [x] DeleteTopics (v0 only)
- [x] ListOffsets (v0 only)
- [ ] etcd topic/partition management

### Milestone 6.5: Ops & Scaling APIs

- [ ] DescribeGroups (API 15) for ops debugging parity
- [ ] ListGroups (API 16) for cluster-wide consumer visibility
- [ ] OffsetForLeaderEpoch (API 23) to unblock recovery-aware consumers
- [ ] DescribeConfigs (API 32) with full coverage of broker/topic configs
- [ ] AlterConfigs (API 33) for runtime tuning without restarts
- [ ] CreatePartitions (API 37) so partitions can scale without recreation
- [ ] DeleteGroups (API 42) for consumer group cleanup

### Milestone 7: Kubernetes Operator

- [x] CRD definitions
- [x] Cluster reconciler
- [x] Topic reconciler
- [x] Broker deployment management
- [x] HPA configuration

### Milestone 8: Observability

- [x] Prometheus metrics
- [ ] Structured logging
- [x] Health endpoints
- [ ] Grafana dashboard templates

### Milestone 8.5: Console UI Access

- [x] UI login screen with env-gated auth (no defaults)

### Milestone 9: Testing & Hardening

- [x] Integration test suite
- [x] Kafka client compatibility tests
- [x] Chaos testing (pod failures, S3 latency)
- [ ] Performance benchmarks

### Milestone 10: Production Readiness

- [x] Helm chart
- [x] Documentation
- [x] CI/CD pipeline
- [ ] Security review (TLS, auth)

### Gap Backlog (Validated)

- [x] Decide etcd schema direction (snapshot-only for now; defer per-key registry/assignment keys)
- [x] Rebuild partition logs on broker restart by listing S3 segments + loading indexes
- [x] Align fetch request support with v13 (spec)
- [x] Align S3 key layout with namespace support and use index-based range reads
- [x] Persist consumer group metadata (not just offsets) in etcd or document in-memory limitations
- [x] Define etcd HA requirements (dedicated cluster, SSD storage, odd quorum) and reflect in ops docs
- [x] Honor `KAFSCALE_OPERATOR_ETCD_ENDPOINTS` when cluster spec omits endpoints
- [x] Auto-provision a 3-node etcd StatefulSet when no endpoints are configured
- [x] Add etcd snapshot backups to S3 + surface snapshot status
- [x] Fix env var mismatches (`KAFSCALE_CACHE_SIZE` vs `KAFSCALE_CACHE_BYTES`, segment/flush vars)
- [ ] Implement Milestone 6.5 Ops APIs (DescribeGroups/ListGroups/OffsetForLeaderEpoch/DescribeConfigs/AlterConfigs/CreatePartitions/DeleteGroups)
- [ ] Expand Prometheus metrics to match observability spec
- [x] E2E: broker port sanity (assert 9092/9093 reachability via Service/port-forward)
- [x] E2E: S3 durability (produce, restart broker, consume from earliest to verify S3-backed recovery)
- [x] E2E: S3 outage handling (stop MinIO, assert produces fail, recover on restart)
- [x] E2E: operator leader election resilience (delete leader twice, ensure reconciliation continues)
- [x] E2E: etcd member loss (delete one pod, snapshots still run)
- [x] E2E: snapshot status metrics/conditions (force failure and confirm EtcdSnapshot/EtcdSnapshotAccess updates)
- [ ] E2E: multi-segment restart durability (produce enough to create multiple segments, verify post-restart fetch)

## Appendix A: S3 Cost Estimation

### Assumptions

- 100 GB/day ingestion
- 7-day retention
- 4 MB average segment size
- 3 broker pods

### Monthly Costs (US-East-1)

| Item | Calculation | Cost |
|------|-------------|------|
| Storage | 700 GB × $0.023/GB | $16.10 |
| PUT requests | 25,000/day × 30 × $0.005/1000 | $3.75 |
| GET requests | 100,000/day × 30 × $0.0004/1000 | $1.20 |
| Data transfer (in-region) | Free | $0 |
| **Total S3** | | **~$21/month** |

### Comparison

- Confluent Cloud Basic: ~$200+/month for similar throughput
- Self-managed Kafka: 3× m5.large + EBS = ~$400/month
- **Kafscale**: $21 S3 + 3× t3.medium (~$90) = **~$111/month**
