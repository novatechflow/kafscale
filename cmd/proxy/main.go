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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/novatechflow/kafscale/pkg/metadata"
	"github.com/novatechflow/kafscale/pkg/protocol"
)

const (
	defaultProxyAddr = ":9092"
)

type proxy struct {
	addr           string
	advertisedHost string
	advertisedPort int32
	store          metadata.Store
	backends       []string
	logger         *slog.Logger
	rr             uint32
	dialTimeout    time.Duration
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	addr := envOrDefault("KAFSCALE_PROXY_ADDR", defaultProxyAddr)
	advertisedHost := strings.TrimSpace(os.Getenv("KAFSCALE_PROXY_ADVERTISED_HOST"))
	advertisedPort := envPort("KAFSCALE_PROXY_ADVERTISED_PORT", portFromAddr(addr, 9092))
	backends := splitCSV(os.Getenv("KAFSCALE_PROXY_BACKENDS"))

	store, err := buildMetadataStore(ctx)
	if err != nil {
		logger.Error("metadata store init failed", "error", err)
		os.Exit(1)
	}
	if store == nil {
		logger.Error("KAFSCALE_PROXY_ETCD_ENDPOINTS not set; proxy cannot build metadata responses")
		os.Exit(1)
	}

	if advertisedHost == "" {
		logger.Warn("KAFSCALE_PROXY_ADVERTISED_HOST not set; clients may not resolve the proxy address")
	}

	p := &proxy{
		addr:           addr,
		advertisedHost: advertisedHost,
		advertisedPort: advertisedPort,
		store:          store,
		backends:       backends,
		logger:         logger,
		dialTimeout:    5 * time.Second,
	}
	if err := p.listenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("proxy server error", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envPort(key string, fallback int) int32 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return int32(fallback)
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		return int32(fallback)
	}
	return int32(parsed)
}

func portFromAddr(addr string, fallback int) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fallback
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fallback
	}
	return port
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val != "" {
			out = append(out, val)
		}
	}
	return out
}

func buildMetadataStore(ctx context.Context) (metadata.Store, error) {
	cfg, ok := proxyEtcdConfigFromEnv()
	if !ok {
		return nil, nil
	}
	return metadata.NewEtcdStore(ctx, metadata.ClusterMetadata{}, cfg)
}

func proxyEtcdConfigFromEnv() (metadata.EtcdStoreConfig, bool) {
	endpoints := strings.TrimSpace(os.Getenv("KAFSCALE_PROXY_ETCD_ENDPOINTS"))
	if endpoints == "" {
		return metadata.EtcdStoreConfig{}, false
	}
	return metadata.EtcdStoreConfig{
		Endpoints: strings.Split(endpoints, ","),
		Username:  os.Getenv("KAFSCALE_PROXY_ETCD_USERNAME"),
		Password:  os.Getenv("KAFSCALE_PROXY_ETCD_PASSWORD"),
	}, true
}

func (p *proxy) listenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", p.addr)
	if err != nil {
		return err
	}
	p.logger.Info("proxy listening", "addr", ln.Addr().String())

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				p.logger.Warn("accept temporary error", "error", err)
				continue
			}
			return err
		}
		go p.handleConnection(ctx, conn)
	}
}

func (p *proxy) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	backendConn, backendAddr, err := p.connectBackend(ctx)
	if err != nil {
		p.logger.Error("backend connect failed", "error", err)
		return
	}
	defer backendConn.Close()

	for {
		frame, err := protocol.ReadFrame(conn)
		if err != nil {
			return
		}
		header, _, err := protocol.ParseRequestHeader(frame.Payload)
		if err != nil {
			p.logger.Warn("parse request header failed", "error", err)
			return
		}

		switch header.APIKey {
		case protocol.APIKeyMetadata:
			resp, err := p.handleMetadata(ctx, header, frame.Payload)
			if err != nil {
				p.logger.Warn("metadata handling failed", "error", err)
				return
			}
			if err := protocol.WriteFrame(conn, resp); err != nil {
				p.logger.Warn("write metadata response failed", "error", err)
				return
			}
			continue
		case protocol.APIKeyFindCoordinator:
			resp, err := p.handleFindCoordinator(header)
			if err != nil {
				p.logger.Warn("find coordinator handling failed", "error", err)
				return
			}
			if err := protocol.WriteFrame(conn, resp); err != nil {
				p.logger.Warn("write coordinator response failed", "error", err)
				return
			}
			continue
		default:
		}

		resp, err := p.forwardToBackend(ctx, backendConn, backendAddr, frame.Payload)
		if err != nil {
			backendConn.Close()
			backendConn, backendAddr, err = p.connectBackend(ctx)
			if err != nil {
				p.logger.Warn("backend reconnect failed", "error", err)
				return
			}
			resp, err = p.forwardToBackend(ctx, backendConn, backendAddr, frame.Payload)
			if err != nil {
				p.logger.Warn("backend forward failed", "error", err)
				return
			}
		}
		if err := protocol.WriteFrame(conn, resp); err != nil {
			p.logger.Warn("write response failed", "error", err)
			return
		}
	}
}

func (p *proxy) handleMetadata(ctx context.Context, header *protocol.RequestHeader, payload []byte) ([]byte, error) {
	_, req, err := protocol.ParseRequest(payload)
	if err != nil {
		return nil, err
	}
	metaReq, ok := req.(*protocol.MetadataRequest)
	if !ok {
		return nil, fmt.Errorf("unexpected metadata request type %T", req)
	}

	meta, err := p.loadMetadata(ctx, metaReq)
	if err != nil {
		return nil, err
	}
	resp := buildProxyMetadataResponse(meta, header.CorrelationID, header.APIVersion, p.advertisedHost, p.advertisedPort)
	return protocol.EncodeMetadataResponse(resp, header.APIVersion)
}

func (p *proxy) handleFindCoordinator(header *protocol.RequestHeader) ([]byte, error) {
	resp := &protocol.FindCoordinatorResponse{
		CorrelationID: header.CorrelationID,
		ThrottleMs:    0,
		ErrorCode:     protocol.NONE,
		NodeID:        0,
		Host:          p.advertisedHost,
		Port:          p.advertisedPort,
		ErrorMessage:  nil,
	}
	return protocol.EncodeFindCoordinatorResponse(resp, header.APIVersion)
}

func (p *proxy) loadMetadata(ctx context.Context, req *protocol.MetadataRequest) (*metadata.ClusterMetadata, error) {
	useIDs := false
	zeroID := [16]byte{}
	for _, id := range req.TopicIDs {
		if id != zeroID {
			useIDs = true
			break
		}
	}
	if !useIDs {
		return p.store.Metadata(ctx, req.Topics)
	}
	all, err := p.store.Metadata(ctx, nil)
	if err != nil {
		return nil, err
	}
	index := make(map[[16]byte]protocol.MetadataTopic, len(all.Topics))
	for _, topic := range all.Topics {
		index[topic.TopicID] = topic
	}
	filtered := make([]protocol.MetadataTopic, 0, len(req.TopicIDs))
	for _, id := range req.TopicIDs {
		if id == zeroID {
			continue
		}
		if topic, ok := index[id]; ok {
			filtered = append(filtered, topic)
		} else {
			filtered = append(filtered, protocol.MetadataTopic{
				ErrorCode: protocol.UNKNOWN_TOPIC_ID,
				TopicID:   id,
			})
		}
	}
	return &metadata.ClusterMetadata{
		Brokers:      all.Brokers,
		ClusterID:    all.ClusterID,
		ControllerID: all.ControllerID,
		Topics:       filtered,
	}, nil
}

func buildProxyMetadataResponse(meta *metadata.ClusterMetadata, correlationID int32, version int16, host string, port int32) *protocol.MetadataResponse {
	brokers := []protocol.MetadataBroker{{
		NodeID: 0,
		Host:   host,
		Port:   port,
	}}
	topics := make([]protocol.MetadataTopic, 0, len(meta.Topics))
	for _, topic := range meta.Topics {
		if topic.ErrorCode != protocol.NONE {
			topics = append(topics, topic)
			continue
		}
		partitions := make([]protocol.MetadataPartition, 0, len(topic.Partitions))
		for _, part := range topic.Partitions {
			partitions = append(partitions, protocol.MetadataPartition{
				ErrorCode:      part.ErrorCode,
				PartitionIndex: part.PartitionIndex,
				LeaderID:       0,
				LeaderEpoch:    part.LeaderEpoch,
				ReplicaNodes:   []int32{0},
				ISRNodes:       []int32{0},
			})
		}
		topics = append(topics, protocol.MetadataTopic{
			ErrorCode:  topic.ErrorCode,
			Name:       topic.Name,
			TopicID:    topic.TopicID,
			IsInternal: topic.IsInternal,
			Partitions: partitions,
		})
	}
	return &protocol.MetadataResponse{
		CorrelationID: correlationID,
		ThrottleMs:    0,
		Brokers:       brokers,
		ClusterID:     meta.ClusterID,
		ControllerID:  0,
		Topics:        topics,
	}
}

func (p *proxy) connectBackend(ctx context.Context) (net.Conn, string, error) {
	backends, err := p.currentBackends(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(backends) == 0 {
		return nil, "", errors.New("no backends available")
	}
	index := atomic.AddUint32(&p.rr, 1)
	addr := backends[int(index)%len(backends)]
	dialer := net.Dialer{Timeout: p.dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, "", err
	}
	return conn, addr, nil
}

func (p *proxy) currentBackends(ctx context.Context) ([]string, error) {
	if len(p.backends) > 0 {
		return p.backends, nil
	}
	meta, err := p.store.Metadata(ctx, nil)
	if err != nil {
		return nil, err
	}
	addrs := make([]string, 0, len(meta.Brokers))
	for _, broker := range meta.Brokers {
		if broker.Host == "" || broker.Port == 0 {
			continue
		}
		addrs = append(addrs, fmt.Sprintf("%s:%d", broker.Host, broker.Port))
	}
	return addrs, nil
}

func (p *proxy) forwardToBackend(ctx context.Context, conn net.Conn, backendAddr string, payload []byte) ([]byte, error) {
	if err := protocol.WriteFrame(conn, payload); err != nil {
		return nil, err
	}
	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return nil, err
	}
	return frame.Payload, nil
}
