// Copyright 2025 Alexander Alten (novatechflow), NovaTechflow (novatechflow.com).
// This project is supported and financed by Scalytics, Inc. (www.scalytics.io).
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"

	"github.com/novatechflow/kafscale/pkg/metadata"
	"github.com/novatechflow/kafscale/pkg/protocol"
)

func TestBuildProxyMetadataResponseRewritesBrokers(t *testing.T) {
	meta := &metadata.ClusterMetadata{
		Brokers: []protocol.MetadataBroker{
			{NodeID: 1, Host: "broker-1", Port: 9092},
		},
		Topics: []protocol.MetadataTopic{
			{
				Name:    "orders",
				TopicID: metadata.TopicIDForName("orders"),
				Partitions: []protocol.MetadataPartition{
					{
						PartitionIndex: 0,
						LeaderID:       1,
						ReplicaNodes:   []int32{1, 2},
						ISRNodes:       []int32{1},
					},
				},
			},
		},
	}

	resp := buildProxyMetadataResponse(meta, 12, 12, "proxy.example.com", 9092)
	if len(resp.Brokers) != 1 {
		t.Fatalf("expected 1 broker, got %d", len(resp.Brokers))
	}
	if resp.Brokers[0].NodeID != 0 || resp.Brokers[0].Host != "proxy.example.com" || resp.Brokers[0].Port != 9092 {
		t.Fatalf("unexpected broker: %+v", resp.Brokers[0])
	}
	if len(resp.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(resp.Topics))
	}
	part := resp.Topics[0].Partitions[0]
	if part.LeaderID != 0 {
		t.Fatalf("expected leader 0, got %d", part.LeaderID)
	}
	if len(part.ReplicaNodes) != 1 || part.ReplicaNodes[0] != 0 {
		t.Fatalf("expected replica nodes [0], got %+v", part.ReplicaNodes)
	}
	if len(part.ISRNodes) != 1 || part.ISRNodes[0] != 0 {
		t.Fatalf("expected ISR nodes [0], got %+v", part.ISRNodes)
	}
}

func TestBuildProxyMetadataResponsePreservesTopicErrors(t *testing.T) {
	meta := &metadata.ClusterMetadata{
		Topics: []protocol.MetadataTopic{
			{
				Name:      "missing",
				ErrorCode: protocol.UNKNOWN_TOPIC_OR_PARTITION,
			},
		},
	}
	resp := buildProxyMetadataResponse(meta, 1, 1, "proxy", 9092)
	if len(resp.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(resp.Topics))
	}
	if resp.Topics[0].ErrorCode != protocol.UNKNOWN_TOPIC_OR_PARTITION {
		t.Fatalf("expected error code %d, got %d", protocol.UNKNOWN_TOPIC_OR_PARTITION, resp.Topics[0].ErrorCode)
	}
}
