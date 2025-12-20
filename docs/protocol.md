# Kafka Protocol Support

Kafscale implements a focused subset of the Kafka protocol. Versions below reflect what the broker advertises in ApiVersions today.

## Supported

| API Key | Name | Version | Notes |
|---------|------|---------|-------|
| 0 | Produce | 0-9 | Core produce path |
| 1 | Fetch | 11-13 | Core consume path |
| 2 | ListOffsets | 0 | Required for consumers (v0 only) |
| 3 | Metadata | 0-12 | Topic/broker discovery |
| 8 | OffsetCommit | 3 | Consumer group tracking (v3 only) |
| 9 | OffsetFetch | 5 | Consumer group tracking (v5 only) |
| 10 | FindCoordinator | 3 | Group coordinator lookup (v3 only) |
| 11 | JoinGroup | 4 | Consumer group membership (v4 only) |
| 12 | Heartbeat | 4 | Consumer liveness (v4 only) |
| 13 | LeaveGroup | 4 | Graceful consumer shutdown (v4 only) |
| 14 | SyncGroup | 4 | Partition assignment (v4 only) |
| 18 | ApiVersions | 0 | Client capability negotiation (v0 only) |
| 19 | CreateTopics | 0 | Topic management (v0 only) |
| 20 | DeleteTopics | 0 | Topic management (v0 only) |

## Planned (Not Yet Supported)

| API Key | Name | Notes |
|---------|------|-------|
| 15 | DescribeGroups | Ops debugging |
| 16 | ListGroups | Ops debugging |
| 23 | OffsetForLeaderEpoch | Safe consumer recovery |
| 32 | DescribeConfigs | Read topic/broker config |
| 33 | AlterConfigs | Runtime config changes |
| 37 | CreatePartitions | Scale partitions |
| 42 | DeleteGroups | Consumer group cleanup |

## Explicitly Unsupported

| API Key | Name | Reason |
|---------|------|--------|
| 4 | LeaderAndIsr | Internal Kafka protocol |
| 5 | StopReplica | No replication (S3 durability) |
| 6 | UpdateMetadata | Internal Kafka protocol |
| 7 | ControlledShutdown | Kubernetes handles lifecycle |
| 21 | DeleteRecords | S3 lifecycle handles retention |
| 22 | InitProducerId | Transactions not supported |
| 24 | AddPartitionsToTxn | Transactions not supported |
| 25 | AddOffsetsToTxn | Transactions not supported |
| 26 | EndTxn | Transactions not supported |
| 46 | ListPartitionReassignments | No manual reassignment |
| 47 | OffsetDelete | S3 lifecycle handles cleanup |
| 48-49 | DescribeClientQuotas/AlterClientQuotas | Quotas deferred |
| 50-56 | KRaft APIs | Using etcd |
| 57 | UpdateFeatures | Feature flags deferred |
| 65-67 | Transaction APIs | Transactions not supported |

## Authentication Roadmap

| Version | Auth Mechanism | Use Case |
|---------|---------------|----------|
| v1.0 | None | Internal/dev clusters |
| v1.1 | SASL/PLAIN | Username/password |
| v1.1 | SASL/SCRAM-SHA-256/512 | Username/password + challenge-response |
| v2.0 | SASL/OAUTHBEARER | Enterprise SSO |
| v2.0 | mTLS | Certificate-based auth |

Until auth lands, Kafscale responds to SASL handshake attempts with `UNSUPPORTED_SASL_MECHANISM` (error code 33).
