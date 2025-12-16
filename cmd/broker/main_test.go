package main

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/alo/kafscale/pkg/metadata"
	"github.com/alo/kafscale/pkg/protocol"
	"github.com/alo/kafscale/pkg/storage"
)

func TestHandleProduceAckAll(t *testing.T) {
	store := metadata.NewInMemoryStore(defaultMetadata())
	handler := newHandler(store, storage.NewMemoryS3Client())

	req := &protocol.ProduceRequest{
		Acks:      -1,
		TimeoutMs: 1000,
		Topics: []protocol.ProduceTopic{
			{
				Name: "orders",
				Partitions: []protocol.ProducePartition{
					{
						Partition: 0,
						Records:   testBatchBytes(0, 0, 1),
					},
				},
			},
		},
	}

	resp, err := handler.handleProduce(context.Background(), &protocol.RequestHeader{CorrelationID: 1}, req)
	if err != nil {
		t.Fatalf("handleProduce: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response for acks=-1")
	}

	offset, err := store.NextOffset(context.Background(), "orders", 0)
	if err != nil {
		t.Fatalf("NextOffset: %v", err)
	}
	if offset != 1 {
		t.Fatalf("expected offset advanced to 1 got %d", offset)
	}
}

func TestHandleProduceAckZero(t *testing.T) {
	store := metadata.NewInMemoryStore(defaultMetadata())
	handler := newHandler(store, storage.NewMemoryS3Client())

	req := &protocol.ProduceRequest{
		Acks:      0,
		TimeoutMs: 1000,
		Topics: []protocol.ProduceTopic{
			{
				Name: "orders",
				Partitions: []protocol.ProducePartition{
					{
						Partition: 0,
						Records:   testBatchBytes(0, 0, 1),
					},
				},
			},
		},
	}

	resp, err := handler.handleProduce(context.Background(), &protocol.RequestHeader{CorrelationID: 1}, req)
	if err != nil {
		t.Fatalf("handleProduce: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected no response for acks=0")
	}
}

func TestHandleFetch(t *testing.T) {
	store := metadata.NewInMemoryStore(defaultMetadata())
	handler := newHandler(store, storage.NewMemoryS3Client())

	produceReq := &protocol.ProduceRequest{
		Acks:      -1,
		TimeoutMs: 1000,
		Topics: []protocol.ProduceTopic{
			{
				Name: "orders",
				Partitions: []protocol.ProducePartition{
					{
						Partition: 0,
						Records:   testBatchBytes(0, 0, 1),
					},
				},
			},
		},
	}
	if _, err := handler.handleProduce(context.Background(), &protocol.RequestHeader{CorrelationID: 1}, produceReq); err != nil {
		t.Fatalf("handleProduce: %v", err)
	}

	fetchReq := &protocol.FetchRequest{
		Topics: []protocol.FetchTopicRequest{
			{
				Name: "orders",
				Partitions: []protocol.FetchPartitionRequest{
					{
						Partition:   0,
						FetchOffset: 0,
						MaxBytes:    1024,
					},
				},
			},
		},
	}

	resp, err := handler.handleFetch(context.Background(), &protocol.RequestHeader{CorrelationID: 2}, fetchReq)
	if err != nil {
		t.Fatalf("handleFetch: %v", err)
	}
	if len(resp) == 0 {
		t.Fatalf("expected non-empty response for fetch")
	}
}

func testBatchBytes(baseOffset int64, lastOffsetDelta int32, messageCount int32) []byte {
	data := make([]byte, 70)
	binary.BigEndian.PutUint64(data[0:8], uint64(baseOffset))
	binary.BigEndian.PutUint32(data[23:27], uint32(lastOffsetDelta))
	binary.BigEndian.PutUint32(data[57:61], uint32(messageCount))
	return data
}
