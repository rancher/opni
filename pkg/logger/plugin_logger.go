package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"sync"
	"testing"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/spf13/afero"
	"google.golang.org/protobuf/proto"
)

const (
	pluginGroupPrefix                         = "plugin"
	forwardedPluginPrefix                     = "plugin."
	pluginLoggerKey       pluginLoggerKeyType = "plugin_logger"
	pluginModeKey         pluginModeKeyType   = "plugin_logger_mode"
	pluginAgentKey        pluginAgentKeyType  = "plugin_logger_agent"
)

type (
	pluginLoggerKeyType string
	pluginModeKeyType   string
	pluginAgentKeyType  string
)

func NewPluginLogger(ctx context.Context, opts ...LoggerOption) *slog.Logger {
	options := &LoggerOptions{
		Level:     DefaultLogLevel,
		AddSource: true,
	}
	options.apply(opts...)

	if options.Writer == nil {
		options.Writer = newPluginWriter(ctx)
	}

	return slog.New(newProtoHandler(options.Writer, ConfigureProtoOptions(options))).WithGroup(pluginGroupPrefix)
}

func WithPluginLogger(ctx context.Context, lg *slog.Logger) context.Context {
	return context.WithValue(ctx, pluginLoggerKey, lg)
}

func PluginLoggerFromContext(ctx context.Context) *slog.Logger {
	logger := ctx.Value(pluginLoggerKey)
	if logger == nil {
		return NewPluginLogger(ctx)
	}
	return logger.(*slog.Logger)
}

func ReadFile(filename string) afero.File {
	f, err := logFs.OpenFile(filename, os.O_CREATE|os.O_RDONLY, 0666)
	if err != nil {
		panic(err)
	}
	return f
}

func WriteOnlyFile(filename string) *FileWriter {
	fileWriter, ok := fileDesc.Load(filename)
	if ok {
		return fileWriter
	}

	newFile, err := logFs.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	newFileWriter := newFileWriter(newFile)
	fileDesc.Store(filename, newFileWriter)

	return newFileWriter
}

func GetLogFileName(agentId string) string {
	return fmt.Sprintf("plugin_%s_%s", meta.ModeAgent, agentId)
}

func WithMode(ctx context.Context, mode meta.PluginMode) context.Context {
	return context.WithValue(ctx, pluginModeKey, mode)
}

func WithAgentId(ctx context.Context, agentId string) context.Context {
	return context.WithValue(ctx, pluginAgentKey, agentId)
}

// writer used for agent loggers and plugin loggers
type RemotePluginWriter struct {
	textWriter  *slog.Logger
	fileWriter  *FileWriter
	protoWriter io.Writer
}

func newPluginWriter(ctx context.Context) *RemotePluginWriter {
	if isInProcessPluginLogger() {
		mode := getMode(ctx)
		if mode == meta.ModeAgent {
			return NewPluginFileWriter(ctx)
		} else {
			return newTestGatewayPluginWriter()
		}
	} else {
		return newSubprocPluginWriter()
	}
}

func NewPluginFileWriter(ctx context.Context) *RemotePluginWriter {
	return &RemotePluginWriter{
		textWriter:  New(WithWriter(os.Stderr), WithDisableCaller()),
		fileWriter:  WriteOnlyFile(GetLogFileName(getAgentId(ctx))),
		protoWriter: io.Discard,
	}
}

func newTestGatewayPluginWriter() *RemotePluginWriter {
	return &RemotePluginWriter{
		textWriter:  New(WithWriter(os.Stderr), WithDisableCaller()),
		fileWriter:  newFileWriter(nil),
		protoWriter: io.Discard,
	}
}

func newSubprocPluginWriter() *RemotePluginWriter {
	return &RemotePluginWriter{
		textWriter:  New(WithWriter(io.Discard), WithDisableCaller()),
		fileWriter:  newFileWriter(nil),
		protoWriter: os.Stderr,
	}
}

func (w *RemotePluginWriter) Write(b []byte) (int, error) {
	if w.fileWriter == nil || w.textWriter == nil {
		return 0, nil
	}

	n, err := w.writeProtoToText(b)
	if err != nil {
		// not a proto message. log as is
		w.textWriter.Info(string(b))
		return n, nil
	}

	n, err = w.fileWriter.Write(b)
	if err != nil {
		return n, err
	}
	n, err = w.protoWriter.Write(b)
	if err != nil {
		return n, err
	}
	return n, err
}

func CloseLogStreaming(agentId string) {
	fileWriter, ok := fileDesc.Load(GetLogFileName(agentId))
	if ok {
		fileWriter.file.Close()
	}
}

func (w *RemotePluginWriter) writeProtoToText(b []byte) (int, error) {
	unsafeN := len(b)
	record := &controlv1.StructuredLogRecord{}

	if unsafeN < 4 {
		return 0, io.ErrUnexpectedEOF
	}

	size := uint32(b[0]) |
		uint32(b[1])<<8 |
		uint32(b[2])<<16 |
		uint32(b[3])<<24

	invalidHeader := size > 65535
	if invalidHeader {
		return 0, io.ErrUnexpectedEOF
	}

	if err := proto.Unmarshal(b[4:size+4], record); err != nil {
		w.textWriter.Error("malformed plugin log", "log", b)
		return 0, err
	}

	lg := w.textWriter.WithGroup(record.GetName())

	attrs := []any{slog.SourceKey, record.GetSource()}
	for _, attr := range record.GetAttributes() {
		attrs = append(attrs, attr.Key, attr.Value)
	}

	switch record.GetLevel() {
	case levelString[0]:
		lg.Debug(record.Message, attrs...)
	case levelString[1]:
		lg.Info(record.Message, attrs...)
	case levelString[2]:
		lg.Warn(record.Message, attrs...)
	case levelString[3]:
		lg.Error(record.Message, attrs...)
	default:
		lg.Debug(record.Message, attrs...)
	}

	return int(size), nil
}

// stores agent and agent plugin logs, retrieved with debug cli
type FileWriter struct {
	file afero.File
	mu   *sync.RWMutex
}

func newFileWriter(f afero.File) *FileWriter {
	return &FileWriter{
		file: f,
		mu:   &sync.RWMutex{},
	}
}

func (f *FileWriter) Write(b []byte) (int, error) {
	if f.file == nil {
		return 0, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Write(b)
}

func getAgentId(ctx context.Context) string {
	id := ctx.Value(pluginAgentKey)
	if id != nil {
		return id.(string)
	}

	return ""
}

func isInProcessPluginLogger() bool {
	return getModuleBasename() == "testenv" || testing.Testing()
}

func getModuleBasename() string {
	md := meta.ReadMetadata()
	return path.Base(md.Module)
}

func getMode(ctx context.Context) meta.PluginMode {
	mode := ctx.Value(pluginModeKey)

	if mode != nil {
		return mode.(meta.PluginMode)
	}
	return meta.ModeGateway
}
