# Kafscale Roadmap

This roadmap tracks completed work and open gaps. It is intentionally high level; detailed specs live in `kafscale-spec.md`.

## Milestones (Completed)

- Core protocol parsing and metadata support
- Produce and fetch paths with S3-backed durability
- Consumer group coordination with offset and group persistence
- Kubernetes operator with managed etcd + snapshot backups
- End-to-end tests for broker durability and operator resilience

## Milestones (Open)

### Admin Operations

- etcd topic/partition management

### Ops and Scaling APIs

- DescribeGroups/ListGroups (ops visibility)
- OffsetForLeaderEpoch (consumer recovery)
- DescribeConfigs/AlterConfigs (runtime tuning)
- CreatePartitions (scale without recreation)
- DeleteGroups (group cleanup)

### Observability

- Structured logging
- Grafana dashboard templates
- Expanded Prometheus metrics

### Testing and Hardening

- Performance benchmarks
- Multi-segment restart durability e2e
- Security review (TLS/auth)

## Gap Backlog (Open)

- Implement Milestone 6.5 Ops APIs
- Expand Prometheus metrics to match observability spec
- E2E: multi-segment restart durability
