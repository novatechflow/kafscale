# Kafscale Overview

Kafscale is a Kubernetes-native, S3-backed Kafka-compatible streaming platform focused on durable message delivery without the operational overhead of a full Kafka cluster.

## Project Philosophy

Most Kafka deployments serve as durable pipes moving data from point A to points B through N. They do not require sub-millisecond latency, exactly-once transactions, or compacted topics. They need messages in, messages out, consumer tracking, and the confidence that data will not be lost.

Kafscale trades latency for operational simplicity. Brokers are stateless. S3 is the source of truth. Kubernetes handles scaling and failover. The result is a system that can scale down cleanly and does not require dedicated ops expertise.

## What Kafscale Optimizes For

- Operational simplicity on Kubernetes.
- Durable storage via S3.
- Compatibility with common Kafka client workflows.
- Clear operational boundaries (broker compute vs S3 durability).

## Non-Goals (Current)

- Transactions or exactly-once semantics.
- Log compaction.
- KRaft or Kafka-internal replication APIs.

## Where to Go Next

- `docs/quickstart.md` for installation.
- `docs/architecture.md` for system internals.
- `docs/protocol.md` for Kafka protocol coverage.
- `kafscale-spec.md` for the technical specification.
