---
layout: doc
title: Rationale
description: Why KafScale uses stateless brokers and object storage instead of traditional Kafka clusters.
permalink: /rationale/
sidebar: false
---

# Rationale

KafScale exists because the original assumptions behind Kafka brokers no longer hold for a large class of modern workloads. This page explains those assumptions, what changed, and why KafScale is designed the way it is.

This is not a comparison page and not a feature list. It documents the architectural reasoning behind the system.

---

## The original Kafka assumptions

Kafka was designed in a world where durability lived on local disks attached to long-running servers. Brokers owned their data. Replication, leader election, and partition rebalancing were necessary because broker state was the source of truth.

That model worked well when:

- Disks were the primary durable medium
- Brokers were expected to be long-lived
- Scaling events were rare and manual
- Recovery time could be measured in minutes or hours

Many Kafka deployments today still operate under these assumptions, even when the workload does not require them.

---

## Object storage changes the durability model

Object storage fundamentally changes where durability lives.

Modern object stores provide extremely high durability, elastic capacity, and simple lifecycle management. Once log segments are durable and immutable in object storage, keeping the same data replicated across broker-local disks stops adding resilience and starts adding operational cost.

With object storage:

- Data durability is decoupled from broker lifecycle
- Storage scales independently from compute
- Recovery no longer depends on copying large volumes of data between brokers

This enables a different design space where brokers no longer need to be stateful.

<div class="diagram">
  <div class="diagram-label">What changed</div>
  <svg viewBox="0 0 750 280" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Traditional vs stateless broker model comparison">
    <defs>
      <marker id="ar-rat" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="var(--diagram-stroke)"/></marker>
    </defs>

    <!-- Traditional model -->
    <rect x="20" y="20" width="340" height="200" rx="12" fill="var(--diagram-fill)" stroke="var(--diagram-stroke)" stroke-width="1.5"/>
    <text x="190" y="48" font-size="13" font-weight="600" fill="var(--diagram-text)" text-anchor="middle">Traditional Kafka</text>
    
    <text x="40" y="80" font-size="11" fill="var(--diagram-text)">• Brokers own durable data</text>
    <text x="40" y="102" font-size="11" fill="var(--diagram-text)">• Replication required for durability</text>
    <text x="40" y="124" font-size="11" fill="var(--diagram-text)">• Scaling moves data</text>
    <text x="40" y="146" font-size="11" fill="var(--diagram-text)">• Failures require repair</text>
    <text x="40" y="168" font-size="11" fill="var(--diagram-text)">• Disk management is critical</text>
    
    <rect x="40" y="185" width="300" height="24" rx="6" fill="rgba(248, 113, 113, 0.15)" stroke="#f87171" stroke-width="1"/>
    <text x="190" y="202" font-size="10" fill="#f87171" text-anchor="middle">Stateful brokers = operational complexity</text>

    <!-- Arrow -->
    <path d="M375,120 L405,120" stroke="var(--diagram-stroke)" stroke-width="2" fill="none" marker-end="url(#ar-rat)"/>

    <!-- Stateless model -->
    <rect x="420" y="20" width="310" height="200" rx="12" fill="var(--diagram-fill)" stroke="var(--diagram-stroke)" stroke-width="1.5"/>
    <text x="575" y="48" font-size="13" font-weight="600" fill="var(--diagram-text)" text-anchor="middle">Stateless Brokers (KafScale)</text>
    
    <text x="440" y="80" font-size="11" fill="var(--diagram-text)">• Object storage owns durability</text>
    <text x="440" y="102" font-size="11" fill="var(--diagram-text)">• Replication is implicit in S3</text>
    <text x="440" y="124" font-size="11" fill="var(--diagram-text)">• Scaling adds compute only</text>
    <text x="440" y="146" font-size="11" fill="var(--diagram-text)">• Failures handled by replacement</text>
    <text x="440" y="168" font-size="11" fill="var(--diagram-text)">• Disk management disappears</text>
    
    <rect x="440" y="185" width="270" height="24" rx="6" fill="rgba(52, 211, 153, 0.15)" stroke="#34d399" stroke-width="1"/>
    <text x="575" y="202" font-size="10" fill="#34d399" text-anchor="middle">Stateless brokers = simpler operations</text>

    <!-- Bottom caption -->
    <text x="375" y="260" font-size="11" fill="var(--diagram-label)" text-anchor="middle" font-style="italic">
      When durability moves out of the broker, the operational model changes with it.
    </text>
  </svg>
</div>

---

## Why brokers should be ephemeral

In KafScale, brokers are treated as ephemeral compute.

They serve the Kafka protocol, buffer and batch data, and flush immutable segments to object storage. They do not own durable state. Any broker can serve any partition.

This has several consequences:

- Scaling is a scheduling problem, not a data movement problem
- Broker restarts are cheap and predictable
- Failures are handled by replacement, not repair
- Kubernetes can manage brokers like any other stateless workload

This model matches how modern infrastructure platforms already operate.

<div class="diagram">
  <div class="diagram-label">Design flow</div>
  <svg viewBox="0 0 700 100" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="KafScale design flow">
    <defs>
      <marker id="af-rat" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="#ffb347"/></marker>
    </defs>

    <!-- Durable storage -->
    <rect x="30" y="25" width="180" height="50" rx="10" fill="rgba(255, 179, 71, 0.12)" stroke="#ffb347" stroke-width="1.5"/>
    <text x="120" y="55" font-size="12" font-weight="600" fill="var(--diagram-text)" text-anchor="middle">Durable storage (S3)</text>

    <path d="M215,50 L265,50" stroke="#ffb347" stroke-width="2" fill="none" marker-end="url(#af-rat)"/>

    <!-- Ephemeral compute -->
    <rect x="270" y="25" width="180" height="50" rx="10" fill="rgba(106, 167, 255, 0.2)" stroke="#6aa7ff" stroke-width="1.5"/>
    <text x="360" y="55" font-size="12" font-weight="600" fill="var(--diagram-text)" text-anchor="middle">Ephemeral compute</text>

    <path d="M455,50 L505,50" stroke="#ffb347" stroke-width="2" fill="none" marker-end="url(#af-rat)"/>

    <!-- Simpler operations -->
    <rect x="510" y="25" width="180" height="50" rx="10" fill="rgba(52, 211, 153, 0.15)" stroke="#34d399" stroke-width="1.5"/>
    <text x="600" y="55" font-size="12" font-weight="600" fill="var(--diagram-text)" text-anchor="middle">Simpler operations</text>
  </svg>
</div>

---

## Why self-hosted control planes still matter

Some systems that adopt stateless brokers rely on vendor-managed control planes for metadata and coordination. That can be a good tradeoff for teams that want a fully managed service.

KafScale makes a different choice.

By keeping metadata, offsets, and consumer group state in a self-hosted store, KafScale can run entirely within your own infrastructure boundary. This matters for:

- Regulated and sovereign environments
- Private and air-gapped deployments
- Teams that require open licensing and forkability
- Platforms that want to avoid external control plane dependencies

The goal is not to reject managed services, but to make the architecture usable under stricter constraints.

---

## What KafScale deliberately does not do

KafScale is not trying to replace every Kafka deployment.

It deliberately does not target:

- Sub-10ms end-to-end latency workloads
- Exactly-once transactions
- Compacted topics
- Embedded stream processing inside the broker

Those features increase broker statefulness and operational complexity. For many workloads, they are unnecessary.

KafScale focuses on the common case: durable message transport, replayability, predictable retention, and low operational overhead.

---

## Summary

Stateless brokers backed by object storage are not a trend. They are a correction.

Once durability moves out of the broker, the system can be simpler, cheaper to operate, and easier to scale. KafScale is built on that assumption, while preserving Kafka protocol compatibility and self-hosted operation.

The architecture is inevitable. The design choices are deliberate.