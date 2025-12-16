package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alo/kafscale/pkg/broker"
	"github.com/alo/kafscale/pkg/cache"
	"github.com/alo/kafscale/pkg/metadata"
	"github.com/alo/kafscale/pkg/protocol"
	"github.com/alo/kafscale/pkg/storage"
)

type handler struct {
	apiVersions []protocol.ApiVersion
	store       metadata.Store
	s3          storage.S3Client
	cache       *cache.SegmentCache
	logs        map[string]map[int32]*storage.PartitionLog
	logMu       sync.Mutex
	logConfig   storage.PartitionLogConfig
}

func (h *handler) Handle(ctx context.Context, header *protocol.RequestHeader, req protocol.Request) ([]byte, error) {
	switch req.(type) {
	case *protocol.ApiVersionsRequest:
		return protocol.EncodeApiVersionsResponse(&protocol.ApiVersionsResponse{
			CorrelationID: header.CorrelationID,
			ErrorCode:     0,
			Versions:      h.apiVersions,
		})
	case *protocol.MetadataRequest:
		metaReq := req.(*protocol.MetadataRequest)
		meta, err := h.store.Metadata(ctx, metaReq.Topics)
		if err != nil {
			return nil, fmt.Errorf("load metadata: %w", err)
		}
		return protocol.EncodeMetadataResponse(&protocol.MetadataResponse{
			CorrelationID: header.CorrelationID,
			Brokers:       meta.Brokers,
			ClusterID:     meta.ClusterID,
			ControllerID:  meta.ControllerID,
			Topics:        meta.Topics,
		})
	case *protocol.ProduceRequest:
		return h.handleProduce(ctx, header, req.(*protocol.ProduceRequest))
	case *protocol.FetchRequest:
		return h.handleFetch(ctx, header, req.(*protocol.FetchRequest))
	default:
		return nil, ErrUnsupportedAPI
	}
}

var ErrUnsupportedAPI = fmt.Errorf("unsupported api")

func (h *handler) handleProduce(ctx context.Context, header *protocol.RequestHeader, req *protocol.ProduceRequest) ([]byte, error) {
	topicResponses := make([]protocol.ProduceTopicResponse, 0, len(req.Topics))
	now := time.Now().UnixMilli()

	for _, topic := range req.Topics {
		partitionResponses := make([]protocol.ProducePartitionResponse, 0, len(topic.Partitions))
		for _, part := range topic.Partitions {
			plog, err := h.getPartitionLog(ctx, topic.Name, part.Partition)
			if err != nil {
				log.Printf("partition log init failed: %v", err)
				partitionResponses = append(partitionResponses, protocol.ProducePartitionResponse{
					Partition: part.Partition,
					ErrorCode: -1,
				})
				continue
			}
			batch, err := storage.NewRecordBatchFromBytes(part.Records)
			if err != nil {
				partitionResponses = append(partitionResponses, protocol.ProducePartitionResponse{
					Partition: part.Partition,
					ErrorCode: -1,
				})
				continue
			}
			result, err := plog.AppendBatch(ctx, batch)
			if err != nil {
				partitionResponses = append(partitionResponses, protocol.ProducePartitionResponse{
					Partition: part.Partition,
					ErrorCode: -1,
				})
				continue
			}
			if req.Acks == -1 {
				if err := plog.Flush(ctx); err != nil {
					log.Printf("flush failed topic=%s partition=%d err=%v", topic.Name, part.Partition, err)
					partitionResponses = append(partitionResponses, protocol.ProducePartitionResponse{
						Partition: part.Partition,
						ErrorCode: -1,
					})
					continue
				}
			}
			partitionResponses = append(partitionResponses, protocol.ProducePartitionResponse{
				Partition:       part.Partition,
				ErrorCode:       0,
				BaseOffset:      result.BaseOffset,
				LogAppendTimeMs: now,
				LogStartOffset:  0,
			})
		}
		topicResponses = append(topicResponses, protocol.ProduceTopicResponse{
			Name:       topic.Name,
			Partitions: partitionResponses,
		})
	}

	if req.Acks == 0 {
		return nil, nil
	}

	return protocol.EncodeProduceResponse(&protocol.ProduceResponse{
		CorrelationID: header.CorrelationID,
		Topics:        topicResponses,
		ThrottleMs:    0,
	})
}

func (h *handler) handleFetch(ctx context.Context, header *protocol.RequestHeader, req *protocol.FetchRequest) ([]byte, error) {
	topicResponses := make([]protocol.FetchTopicResponse, 0, len(req.Topics))

	for _, topic := range req.Topics {
		partitionResponses := make([]protocol.FetchPartitionResponse, 0, len(topic.Partitions))
		for _, part := range topic.Partitions {
			plog, err := h.getPartitionLog(ctx, topic.Name, part.Partition)
			if err != nil {
				partitionResponses = append(partitionResponses, protocol.FetchPartitionResponse{
					Partition: part.Partition,
					ErrorCode: -1,
				})
				continue
			}
			records, err := plog.Read(ctx, part.FetchOffset, part.MaxBytes)
			errorCode := int16(0)
			if err != nil {
				if errors.Is(err, storage.ErrOffsetOutOfRange) {
					errorCode = 1
				} else {
					errorCode = -1
				}
			}
			nextOffset, offsetErr := h.store.NextOffset(ctx, topic.Name, part.Partition)
			if offsetErr != nil {
				nextOffset = 0
			}
			highWatermark := nextOffset
			if highWatermark > 0 {
				highWatermark--
			}
			var recordSet []byte
			if errorCode == 0 {
				recordSet = records
			}
			partitionResponses = append(partitionResponses, protocol.FetchPartitionResponse{
				Partition:     part.Partition,
				ErrorCode:     errorCode,
				HighWatermark: highWatermark,
				RecordSet:     recordSet,
			})
		}
		topicResponses = append(topicResponses, protocol.FetchTopicResponse{
			Name:       topic.Name,
			Partitions: partitionResponses,
		})
	}

	return protocol.EncodeFetchResponse(&protocol.FetchResponse{
		CorrelationID: header.CorrelationID,
		Topics:        topicResponses,
		ThrottleMs:    0,
	})
}

func (h *handler) getPartitionLog(ctx context.Context, topic string, partition int32) (*storage.PartitionLog, error) {
	h.logMu.Lock()
	partitions := h.logs[topic]
	if partitions == nil {
		partitions = make(map[int32]*storage.PartitionLog)
		h.logs[topic] = partitions
	}
	if log, ok := partitions[partition]; ok {
		h.logMu.Unlock()
		return log, nil
	}
	nextOffset, err := h.store.NextOffset(ctx, topic, partition)
	if err != nil {
		h.logMu.Unlock()
		return nil, err
	}
	plog := storage.NewPartitionLog(topic, partition, nextOffset, h.s3, h.cache, h.logConfig, func(cbCtx context.Context, artifact *storage.SegmentArtifact) {
		if err := h.store.UpdateOffsets(cbCtx, topic, partition, artifact.LastOffset); err != nil {
			log.Printf("update offsets failed topic=%s partition=%d err=%v", topic, partition, err)
		}
	})
	partitions[partition] = plog
	h.logMu.Unlock()
	return plog, nil
}
func newHandler(store metadata.Store, s3Client storage.S3Client) *handler {
	readAhead := parseEnvInt("KAFSCALE_READAHEAD_SEGMENTS", 2)
	cacheSize := parseEnvInt("KAFSCALE_CACHE_BYTES", 32<<20)
	return &handler{
		apiVersions: []protocol.ApiVersion{
			{APIKey: protocol.APIKeyApiVersion, MinVersion: 0, MaxVersion: 0},
			{APIKey: protocol.APIKeyMetadata, MinVersion: 0, MaxVersion: 0},
			{APIKey: protocol.APIKeyProduce, MinVersion: 9, MaxVersion: 9},
			{APIKey: protocol.APIKeyFetch, MinVersion: 13, MaxVersion: 13},
		},
		store: store,
		s3:    s3Client,
		cache: cache.NewSegmentCache(cacheSize),
		logs:  make(map[string]map[int32]*storage.PartitionLog),
		logConfig: storage.PartitionLogConfig{
			Buffer: storage.WriteBufferConfig{
				MaxBytes:      4 << 20,
				FlushInterval: 500 * time.Millisecond,
			},
			Segment: storage.SegmentWriterConfig{
				IndexIntervalMessages: 100,
			},
			ReadAheadSegments: readAhead,
			CacheEnabled:      true,
		},
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store := buildStore(ctx)
	s3Client := buildS3Client(ctx)

	srv := &broker.Server{
		Addr:    ":19092",
		Handler: newHandler(store, s3Client),
	}
	if err := srv.ListenAndServe(ctx); err != nil {
		log.Fatalf("broker server error: %v", err)
	}
	srv.Wait()
}

func buildS3Client(ctx context.Context) storage.S3Client {
	bucket := os.Getenv("KAFSCALE_S3_BUCKET")
	region := os.Getenv("KAFSCALE_S3_REGION")
	if bucket == "" || region == "" {
		log.Printf("KAFSCALE_S3_BUCKET or region not set, using in-memory S3")
		return storage.NewMemoryS3Client()
	}
	client, err := storage.NewS3Client(ctx, storage.S3Config{
		Bucket:         bucket,
		Region:         region,
		Endpoint:       os.Getenv("KAFSCALE_S3_ENDPOINT"),
		ForcePathStyle: os.Getenv("KAFSCALE_S3_PATH_STYLE") == "true",
		KMSKeyARN:      os.Getenv("KAFSCALE_S3_KMS_ARN"),
	})
	if err != nil {
		log.Printf("failed to create S3 client (%v), falling back to in-memory", err)
		return storage.NewMemoryS3Client()
	}
	return client
}

func defaultMetadata() metadata.ClusterMetadata {
	clusterID := "kafscale-cluster"
	return metadata.ClusterMetadata{
		ControllerID: 1,
		ClusterID:    &clusterID,
		Brokers: []protocol.MetadataBroker{
			{NodeID: 1, Host: "localhost", Port: 19092},
		},
		Topics: []protocol.MetadataTopic{
			{
				ErrorCode: 0,
				Name:      "orders",
				Partitions: []protocol.MetadataPartition{
					{
						ErrorCode:      0,
						PartitionIndex: 0,
						LeaderID:       1,
						ReplicaNodes:   []int32{1},
						ISRNodes:       []int32{1},
					},
				},
			},
		},
	}
}

func buildStore(ctx context.Context) metadata.Store {
	meta := defaultMetadata()
	endpoints := strings.TrimSpace(os.Getenv("KAFSCALE_ETCD_ENDPOINTS"))
	if endpoints == "" {
		return metadata.NewInMemoryStore(meta)
	}
	cfg := metadata.EtcdStoreConfig{
		Endpoints: strings.Split(endpoints, ","),
		Username:  os.Getenv("KAFSCALE_ETCD_USERNAME"),
		Password:  os.Getenv("KAFSCALE_ETCD_PASSWORD"),
	}
	store, err := metadata.NewEtcdStore(ctx, meta, cfg)
	if err != nil {
		log.Printf("failed to init etcd store: %v; falling back to in-memory", err)
		return metadata.NewInMemoryStore(meta)
	}
	log.Printf("using etcd-backed metadata store (%v)", cfg.Endpoints)
	return store
}
func parseEnvInt(name string, fallback int) int {
	if val := strings.TrimSpace(os.Getenv(name)); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return fallback
}
