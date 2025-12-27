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

package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kafscalev1alpha1 "github.com/novatechflow/kafscale/api/v1alpha1"
	"github.com/novatechflow/kafscale/pkg/metadata"
	"github.com/novatechflow/kafscale/pkg/protocol"
)

const (
	operatorEtcdSilenceLogsEnv = "KAFSCALE_OPERATOR_ETCD_SILENCE_LOGS"
)

// SnapshotPublisher translates KafscaleCluster/KafscaleTopic resources into
// the metadata snapshot consumed by brokers.
type SnapshotPublisher struct {
	Client client.Client
}

// NewSnapshotPublisher builds a publisher backed by the provided controller-runtime client.
func NewSnapshotPublisher(c client.Client) *SnapshotPublisher {
	return &SnapshotPublisher{Client: c}
}

// Publish renders the current metadata for the supplied cluster and puts it into etcd.
func (p *SnapshotPublisher) Publish(ctx context.Context, cluster *kafscalev1alpha1.KafscaleCluster, endpoints []string) error {
	var topicList kafscalev1alpha1.KafscaleTopicList
	if err := p.Client.List(ctx, &topicList, client.InNamespace(cluster.Namespace)); err != nil {
		operatorSnapshotResults.WithLabelValues("error").Inc()
		return err
	}
	topics := make([]kafscalev1alpha1.KafscaleTopic, 0, len(topicList.Items))
	for _, topic := range topicList.Items {
		if topic.Spec.ClusterRef == cluster.Name {
			topics = append(topics, topic)
		}
	}
	meta := BuildClusterMetadata(cluster, topics)
	if err := PublishMetadataSnapshot(ctx, endpoints, meta); err != nil {
		operatorSnapshotResults.WithLabelValues("error").Inc()
		return err
	}
	operatorSnapshotResults.WithLabelValues("success").Inc()
	return nil
}

// BuildClusterMetadata converts CRD state into the metadata snapshot consumed by brokers.
func BuildClusterMetadata(cluster *kafscalev1alpha1.KafscaleCluster, topics []kafscalev1alpha1.KafscaleTopic) metadata.ClusterMetadata {
	replicas := int32(1)
	if cluster.Spec.Brokers.Replicas != nil && *cluster.Spec.Brokers.Replicas > 0 {
		replicas = *cluster.Spec.Brokers.Replicas
	}
	brokers := make([]protocol.MetadataBroker, replicas)
	brokerHost := fmt.Sprintf("%s-broker.%s.svc.cluster.local", cluster.Name, cluster.Namespace)
	if strings.TrimSpace(cluster.Spec.Brokers.AdvertisedHost) != "" {
		brokerHost = strings.TrimSpace(cluster.Spec.Brokers.AdvertisedHost)
	}
	brokerPort := int32(9092)
	if cluster.Spec.Brokers.AdvertisedPort != nil && *cluster.Spec.Brokers.AdvertisedPort > 0 {
		brokerPort = *cluster.Spec.Brokers.AdvertisedPort
	}
	for i := int32(0); i < replicas; i++ {
		brokers[i] = protocol.MetadataBroker{
			NodeID: i,
			Host:   brokerHost,
			Port:   brokerPort,
		}
	}
	metaTopics := make([]protocol.MetadataTopic, 0, len(topics))
	replicaIDs := buildReplicaIDs(replicas)
	clusterID := string(cluster.UID)
	var clusterIDPtr *string
	if clusterID != "" {
		clusterIDPtr = &clusterID
	}
	for _, topic := range topics {
		partitions := make([]protocol.MetadataPartition, topic.Spec.Partitions)
		for i := int32(0); i < topic.Spec.Partitions; i++ {
			leader := replicaIDs[0]
			if len(replicaIDs) > 0 {
				leader = replicaIDs[i%int32(len(replicaIDs))]
			}
			partitions[i] = protocol.MetadataPartition{
				PartitionIndex: i,
				LeaderID:       leader,
				ReplicaNodes:   replicaIDs,
				ISRNodes:       replicaIDs,
			}
		}
		metaTopics = append(metaTopics, protocol.MetadataTopic{
			Name:       topic.Name,
			TopicID:    metadata.TopicIDForName(topic.Name),
			IsInternal: false,
			Partitions: partitions,
		})
	}
	return metadata.ClusterMetadata{
		Brokers:      brokers,
		ControllerID: 0,
		Topics:       metaTopics,
		ClusterID:    clusterIDPtr,
	}
}

func buildReplicaIDs(replicaCount int32) []int32 {
	if replicaCount <= 0 {
		return nil
	}
	out := make([]int32, replicaCount)
	for i := int32(0); i < replicaCount; i++ {
		out[i] = i
	}
	return out
}

// PublishMetadataSnapshot writes the provided metadata snapshot into etcd so brokers can consume it.
func PublishMetadataSnapshot(ctx context.Context, endpoints []string, snapshot metadata.ClusterMetadata) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("etcd endpoints required")
	}
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	}
	if parseBoolEnv(operatorEtcdSilenceLogsEnv) {
		cfg.Logger = zap.NewNop()
	}
	cli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer cli.Close()
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	putCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = cli.Put(putCtx, "/kafscale/metadata/snapshot", string(payload))
	return err
}
