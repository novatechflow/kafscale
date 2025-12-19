# Kafscale

Kafscale is a Kubernetes-native, S3-backed Kafka-compatible streaming platform focused on durable message delivery without the operational overhead of a full Kafka cluster.

## Why Kafscale

- Stateless brokers with S3 as the source of truth.
- Kafka protocol support for the common 80% of producer/consumer workflows.
- etcd-backed metadata and consumer offsets (plus group metadata persistence).
- Designed for Kubernetes scale-out and predictable operations.

## Architecture at a Glance

- Brokers handle Kafka protocol traffic and buffer segments in memory.
- S3 stores immutable log segments and index files.
- etcd stores metadata, offsets, and consumer group state.

For deeper design details and architecture diagrams, see `kscale-spec.md`.

## Kafka Protocol Support (Broker-Advertised)

Versions below reflect what the broker advertises in ApiVersions today.

Supported:
- Produce: v0-9
- Fetch: v11-13
- ListOffsets: v0
- Metadata: v0-12
- FindCoordinator: v3
- JoinGroup / SyncGroup / Heartbeat / LeaveGroup: v4
- OffsetCommit: v3
- OffsetFetch: v5
- CreateTopics: v0
- DeleteTopics: v0

Planned (not yet supported):
- DescribeGroups (15), ListGroups (16)
- OffsetForLeaderEpoch (23)
- DescribeConfigs (32), AlterConfigs (33)
- CreatePartitions (37)
- DeleteGroups (42)

Explicitly unsupported:
- Transactions and KRaft APIs
- Replica management internals (LeaderAndIsr, UpdateMetadata, etc.)

## Quickstart

See `docs/user-guide.md` for deployment and usage, and `docs/development.md` for developer workflows.

Common local commands:

```bash
make build
make test
make test-produce-consume
make test-consumer-group
```

## Documentation Map

- `kafscale-spec.md` - architecture overview, protocol coverage, roadmap
- `docs/user-guide.md` - running the platform
- `docs/development.md` - dev workflow and test targets
- `docs/operations.md` - ops guidance and etcd/S3 requirements
- `docs/storage.md` - S3 layout and segment/index details
