# Kafscale Architecture

This document explains how Kafscale is put together and how data flows through the system.

## Components

- **Brokers** handle Kafka protocol traffic, buffer writes, and serve reads.
- **S3** stores immutable log segments and index files (source of truth).
- **etcd** stores metadata, offsets, and consumer group state.
- **Operator** provisions brokers, manages etcd, and applies CRD changes.
- **Console** exposes a UI for ops visibility.

The architecture diagrams and data formats live in `kafscale-spec.md`.

## Write Path (High Level)

1. Producer sends a Produce request to the broker.
2. Broker appends records to the in-memory partition buffer and assigns offsets.
3. Broker flushes segments to S3 based on size/interval thresholds.
4. Broker updates etcd metadata and acknowledges the producer.

## Read Path (High Level)

1. Consumer sends a Fetch request to the broker.
2. Broker resolves segment locations from cached metadata.
3. Broker reads from cache or fetches from S3 using the index file.
4. Broker returns Kafka record batches to the consumer.

## Caching Strategy

Kafscale uses a layered cache:

- **L1 Hot Segment Cache** for recent segments per partition.
- **L2 Index Cache** for index files to keep range reads fast.

If caches miss, the broker fetches from S3.

## S3 Health Backpressure

When the broker detects degraded or unavailable S3 health, it rejects Produce and Fetch requests with backpressure error codes. Producers fail fast so clients can retry; consumers receive fetch errors until S3 recovers.
