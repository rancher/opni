package logger

import (
	"io"
	"log/slog"
	"os"
	"sync"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/spf13/afero"
	"google.golang.org/protobuf/proto"
)

type fileWriter struct {
	file afero.File
	mu   *sync.Mutex
}

func (f fileWriter) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Write(b)
}

// forwards plugin logs to their host process, where they are logged with a logger in the host process
type remotePluginWriter struct {
	logForwarder *slog.Logger
	fileWriter   *fileWriter
	mu           *sync.Mutex
	stderr       io.Writer
}

func NewPluginWriter(agentId string) *remotePluginWriter {
	mu := &sync.Mutex{}
	// textStderr := io.Discard
	// protoStderr := io.Writer(os.Stderr)
	// if sameProcessPluginLoggers {
	textStderr := os.Stderr
	protoStderr := io.Discard
	// }

	return &remotePluginWriter{
		logForwarder: New(WithWriter(textStderr), WithDisableCaller()),
		fileWriter: &fileWriter{
			file: WriteOnlyFile("temp"), //agentId),
			mu:   mu,
		},
		stderr: protoStderr,
		mu:     mu,
	}
}

func (w remotePluginWriter) Write(b []byte) (int, error) {
	if w.fileWriter == nil || w.logForwarder == nil {
		return 0, nil
	}

	n, err := w.logProtoMessage(b)
	if err != nil {
		// not a proto message. log as is
		w.logForwarder.Info(string(b))
		return n, nil
	}

	n, err = w.fileWriter.Write(b)
	w.stderr.Write(b)

	return n, err
}

func (w remotePluginWriter) Close() { // TODO where to close file?
	w.fileWriter.file.Close()
}

func (w remotePluginWriter) logProtoMessage(b []byte) (int, error) {
	n := len(b)
	if n < 5 {
		return 0, io.ErrUnexpectedEOF
	}
	record := &controlv1.StructuredLogRecord{}

	size := uint32(b[0]) |
		uint32(b[1])<<8 |
		uint32(b[2])<<16 |
		uint32(b[3])<<24

	invalidHeader := size > 65536
	if invalidHeader {
		return 0, io.ErrUnexpectedEOF
	}

	if err := proto.Unmarshal(b[4:size+4], record); err != nil {
		w.logForwarder.Error("malformed plugin log", "log", b)
		return 0, err
	}

	lg := w.logForwarder.WithGroup(record.GetName())

	attrs := []any{slog.SourceKey, record.GetSource()}
	for _, attr := range record.GetAttributes() {
		attrs = append(attrs, attr.Key, attr.Value)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

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

	return n, nil
}
