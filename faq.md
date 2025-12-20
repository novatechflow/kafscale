---
layout: doc
title: FAQ
description: Common questions about Kafscale, Kafka compatibility, and S3 durability.
---

# FAQ

## How does this compare to WarpStream/Redpanda/MSK?

See `/comparison/` for a detailed side-by-side comparison.

## What latency should I expect?

Kafscale prioritizes durability and operational simplicity over sub-10ms latency. Expect S3-bound latencies in the hundreds of milliseconds depending on your region and buffering thresholds.

## Can I use existing Kafka clients?

Yes. Kafscale speaks the Kafka wire protocol for core produce/fetch and consumer group APIs. See `/api` for supported keys and versions.

## What happens if S3 goes down?

Brokers shift to `DEGRADED` or `UNAVAILABLE` health states based on S3 error-rate and latency thresholds. Monitor `kafscale_s3_health_state` and plan for retries/backpressure at the client layer.

## Is this production ready?

Kafscale is used in production, but there are no warranties or guarantees. Review `/operations` and `/security` to align it with your operational requirements.
