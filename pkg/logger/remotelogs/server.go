package remotelogs

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"sync"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/spf13/afero"
	"google.golang.org/protobuf/proto"
)

type LogServer struct {
	controlv1.UnsafeLogServer
	LogServerOptions
	logger *slog.Logger

	clientsMu sync.RWMutex
	clients   map[string]controlv1.LogClient
}

type LogServerOptions struct{}

type LogServerOption func(*LogServerOptions)

func (o *LogServerOptions) apply(opts ...LogServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func NewLogServer(opts ...LogServerOption) *LogServer {
	options := &LogServerOptions{}
	options.apply(opts...)

	return &LogServer{
		clients: make(map[string]controlv1.LogClient),
		logger:  logger.New().WithGroup("agent-log-server"),
	}
}

func (ls *LogServer) AddClient(name string, client controlv1.LogClient) {
	ls.clientsMu.Lock()
	defer ls.clientsMu.Unlock()
	ls.clients[name] = client
}

func (ls *LogServer) RemoveClient(name string) {
	ls.clientsMu.Lock()
	defer ls.clientsMu.Unlock()
	delete(ls.clients, name)
}

func (ls *LogServer) GetLogs(ctx context.Context, req *controlv1.LogStreamRequest) (*controlv1.StructuredLogRecords, error) {
	since := req.Since.AsTime()
	until := req.Until.AsTime()
	minLevel := req.Filters.Level
	nameFilters := req.Filters.NamePattern

	f, err := logger.OpenLogFile(cluster.StreamAuthorizedID(ctx))
	if err != nil {
		ls.logger.Error("failed to open log file")
		return nil, err
	}
	defer f.Close()

	var logs []*controlv1.StructuredLogRecord
	for {
		msg, err := ls.getLogMessage(req, f)
		done := err != nil && err == io.EOF
		if done {
			return &controlv1.StructuredLogRecords{
				Items: logs,
			}, nil
		}
		if err != nil {
			return nil, err
		}

		if minLevel != nil && logger.ParseLevel(msg.Level) < slog.Level(*minLevel) {
			continue
		}

		time := msg.Time.AsTime()
		if time.Before(since) || time.After(until) {
			continue
		}

		if nameFilters != nil && !matchesNameFilter(nameFilters, msg.Name) {
			continue
		}

		logs = append(logs, msg)
	}
}

func (ls *LogServer) getLogMessage(req *controlv1.LogStreamRequest, f afero.File) (*controlv1.StructuredLogRecord, error) {
	sizeBuf := make([]byte, 4)
	record := &controlv1.StructuredLogRecord{}
	_, err := io.ReadFull(f, sizeBuf)
	if err != nil {
		return nil, err
	}

	size := int32(sizeBuf[0]) |
		int32(sizeBuf[1])<<8 |
		int32(sizeBuf[2])<<16 |
		int32(sizeBuf[3])<<24

	recordBytes := make([]byte, size)
	_, err = io.ReadFull(f, recordBytes)
	if err != nil {
		return nil, err
	}

	if err := proto.Unmarshal(recordBytes, record); err != nil {
		ls.logger.Error("failed to unmarshal record bytes")
		return nil, err
	}

	return record, nil
}

func matchesNameFilter(patterns []string, name string) bool {
	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, name)
		if matched {
			return true
		}
	}
	return false
}
