---
layout: doc
title: Comparison
description: Compare Kafscale with Kafka-compatible alternatives across architecture, performance, and operations.
---

# Comparison

Kafka-compatible streaming platforms differ in storage architecture, operational footprint, and performance tradeoffs. This view keeps the focus on architecture and operational reality.

<div class="comparison">
  <table class="comparison-table">
    <thead>
      <tr>
        <th>Feature</th>
        <th>Apache Kafka</th>
        <th>Redpanda</th>
        <th>WarpStream</th>
        <th>AutoMQ</th>
        <th class="highlight">Kafscale</th>
      </tr>
    </thead>
    <tbody>
      <tr class="section-row">
        <td colspan="6">Architecture</td>
      </tr>
      <tr>
        <td>Storage backend</td>
        <td>Local disk</td>
        <td>Local disk + tiered</td>
        <td>S3 only</td>
        <td>S3 + EBS WAL</td>
        <td class="highlight"><strong>S3 only</strong></td>
      </tr>
      <tr>
        <td>Broker state</td>
        <td><span class="badge badge-danger">Stateful</span></td>
        <td><span class="badge badge-danger">Stateful</span></td>
        <td><span class="badge badge-success">Stateless</span></td>
        <td><span class="badge badge-success">Stateless</span></td>
        <td class="highlight"><span class="badge badge-success">Stateless</span></td>
      </tr>
      <tr>
        <td>Metadata store</td>
        <td>ZooKeeper/KRaft</td>
        <td>Internal Raft</td>
        <td>Cloud control plane</td>
        <td>KRaft</td>
        <td class="highlight"><strong>etcd</strong></td>
      </tr>
      <tr>
        <td>Kubernetes native</td>
        <td>Community operators</td>
        <td>Operator</td>
        <td>BYOC</td>
        <td>Operator</td>
        <td class="highlight"><strong>CRDs + Operator</strong></td>
      </tr>
      <tr class="section-row">
        <td colspan="6">Performance</td>
      </tr>
      <tr>
        <td>Typical latency</td>
        <td>&lt;10ms</td>
        <td>&lt;10ms</td>
        <td>~400ms</td>
        <td>&lt;100ms</td>
        <td class="highlight"><strong>~500ms</strong></td>
      </tr>
      <tr>
        <td>Rebalancing</td>
        <td><span class="badge badge-danger">Hours</span></td>
        <td><span class="badge badge-warning">Minutes</span></td>
        <td><span class="badge badge-success">Instant</span></td>
        <td><span class="badge badge-success">Seconds</span></td>
        <td class="highlight"><span class="badge badge-success">Instant</span></td>
      </tr>
      <tr class="section-row">
        <td colspan="6">Features</td>
      </tr>
      <tr>
        <td>Kafka protocol</td>
        <td>Native</td>
        <td>Compatible</td>
        <td>Compatible</td>
        <td>Compatible</td>
        <td class="highlight">Compatible</td>
      </tr>
      <tr>
        <td>Transactions / EOS</td>
        <td>Yes</td>
        <td>Yes</td>
        <td>No</td>
        <td>Yes</td>
        <td class="highlight">No (by design)</td>
      </tr>
      <tr>
        <td>Compacted topics</td>
        <td>Yes</td>
        <td>Yes</td>
        <td>No</td>
        <td>Yes</td>
        <td class="highlight">No (by design)</td>
      </tr>
      <tr class="section-row">
        <td colspan="6">Cost & licensing</td>
      </tr>
      <tr>
        <td>License</td>
        <td><span class="badge badge-success">Apache 2.0</span></td>
        <td><span class="badge badge-warning">BSL 1.1</span></td>
        <td><span class="badge badge-neutral">Proprietary</span></td>
        <td><span class="badge badge-warning">BSL</span></td>
        <td class="highlight"><span class="badge badge-success">Apache 2.0</span></td>
      </tr>
      <tr>
        <td>Self-hosted</td>
        <td>Yes</td>
        <td>Yes</td>
        <td>BYOC</td>
        <td>Yes</td>
        <td class="highlight">Yes</td>
      </tr>
      <tr>
        <td>Best for</td>
        <td>General streaming</td>
        <td>Low latency</td>
        <td>Cost-sensitive ETL</td>
        <td>Cloud migration</td>
        <td class="highlight"><strong>ETL, logs, async</strong></td>
      </tr>
    </tbody>
  </table>

  <div class="mobile-cards">
    <div class="product-card highlight">
      <h3>Kafscale</h3>
      <p class="tagline">Stateless Kafka on S3 • Open Source</p>
      <div class="spec-row"><span class="spec-label">Storage</span><span class="spec-value">S3 only</span></div>
      <div class="spec-row"><span class="spec-label">Broker state</span><span class="spec-value">Stateless</span></div>
      <div class="spec-row"><span class="spec-label">Latency</span><span class="spec-value">~500ms</span></div>
      <div class="spec-row"><span class="spec-label">License</span><span class="spec-value">Apache 2.0</span></div>
      <div class="spec-row"><span class="spec-label">Best for</span><span class="spec-value">ETL, logs, async</span></div>
    </div>

    <div class="product-card">
      <h3>Apache Kafka</h3>
      <p class="tagline">The original • Self-managed</p>
      <div class="spec-row"><span class="spec-label">Storage</span><span class="spec-value">Local disk</span></div>
      <div class="spec-row"><span class="spec-label">Broker state</span><span class="spec-value">Stateful</span></div>
      <div class="spec-row"><span class="spec-label">Latency</span><span class="spec-value">&lt;10ms</span></div>
      <div class="spec-row"><span class="spec-label">License</span><span class="spec-value">Apache 2.0</span></div>
      <div class="spec-row"><span class="spec-label">Best for</span><span class="spec-value">General streaming</span></div>
    </div>

    <div class="product-card">
      <h3>Redpanda</h3>
      <p class="tagline">C++ rewrite • Low latency</p>
      <div class="spec-row"><span class="spec-label">Storage</span><span class="spec-value">Local + tiered</span></div>
      <div class="spec-row"><span class="spec-label">Broker state</span><span class="spec-value">Stateful</span></div>
      <div class="spec-row"><span class="spec-label">Latency</span><span class="spec-value">&lt;10ms</span></div>
      <div class="spec-row"><span class="spec-label">License</span><span class="spec-value">BSL 1.1</span></div>
      <div class="spec-row"><span class="spec-label">Best for</span><span class="spec-value">Low latency</span></div>
    </div>

    <div class="product-card">
      <h3>WarpStream</h3>
      <p class="tagline">S3-native • Confluent-owned</p>
      <div class="spec-row"><span class="spec-label">Storage</span><span class="spec-value">S3 only</span></div>
      <div class="spec-row"><span class="spec-label">Broker state</span><span class="spec-value">Stateless</span></div>
      <div class="spec-row"><span class="spec-label">Latency</span><span class="spec-value">~400ms</span></div>
      <div class="spec-row"><span class="spec-label">License</span><span class="spec-value">Proprietary</span></div>
      <div class="spec-row"><span class="spec-label">Best for</span><span class="spec-value">Cost-sensitive ETL</span></div>
    </div>

    <div class="product-card">
      <h3>AutoMQ</h3>
      <p class="tagline">Kafka fork • S3 + WAL</p>
      <div class="spec-row"><span class="spec-label">Storage</span><span class="spec-value">S3 + EBS WAL</span></div>
      <div class="spec-row"><span class="spec-label">Broker state</span><span class="spec-value">Stateless</span></div>
      <div class="spec-row"><span class="spec-label">Latency</span><span class="spec-value">&lt;100ms</span></div>
      <div class="spec-row"><span class="spec-label">License</span><span class="spec-value">BSL</span></div>
      <div class="spec-row"><span class="spec-label">Best for</span><span class="spec-value">Cloud migration</span></div>
    </div>
  </div>
</div>

## Notes

- Cost and latency ranges are approximate and depend on workload, region, and buffering policy.
- Kafscale intentionally omits transactions and compacted topics to optimize for ETL, log aggregation, and async event workloads.
