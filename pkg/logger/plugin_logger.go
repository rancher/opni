package logger

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

const (
	numGoPluginSegments = 4 // hclog timestamp + hclog level + go-plugin name + log message
	logDelimiter        = " "
)

var (
	logLevelPattern   = `\(` + strings.Join(levelString, "|") + `\)`
	pluginNamePattern = pluginGroupPrefix + `\.\w+`
)

// forwards plugin logs to their hosts. Retrieve these logs with the "debug" cli
type remoteFileWriter struct {
	logForwarder *slog.Logger
	w            io.Writer
	mu           *sync.Mutex
}

func InitPluginWriter(agentId string) io.Writer {
	PluginFileWriter.mu.Lock()
	defer PluginFileWriter.mu.Unlock()

	if PluginFileWriter.w != nil {
		return PluginFileWriter.w
	}

	f := WriteOnlyFile(agentId)
	writer := f.(io.Writer)
	PluginFileWriter.w = writer
	PluginFileWriter.logForwarder = newPluginLogForwarder(writer).WithGroup("forwarded")
	return writer
}

func (f remoteFileWriter) Write(b []byte) (int, error) {
	if f.w == nil || f.logForwarder == nil {
		return 0, nil
	}

	// workaround to remove hclog's default DEBUG prefix on all non-hclog logs
	prefixes := strings.SplitN(string(b), logDelimiter, numGoPluginSegments)
	if len(prefixes) != numGoPluginSegments {
		f.logForwarder.Info(string(b))
		return len(b), nil
	}

	forwardedLog := prefixes[numGoPluginSegments-1]

	levelPattern := regexp.MustCompile(logLevelPattern)
	level := levelPattern.FindString(forwardedLog)

	namePattern := regexp.MustCompile(pluginNamePattern)
	loggerName := namePattern.FindString(forwardedLog)

	namedLogger := f.logForwarder.WithGroup(loggerName)

	switch level {
	case levelString[0]:
		namedLogger.Debug(forwardedLog)
	case levelString[1]:
		namedLogger.Info(forwardedLog)
	case levelString[2]:
		namedLogger.Warn(forwardedLog)
	case levelString[3]:
		namedLogger.Error(forwardedLog)
	default:
		namedLogger.Debug(forwardedLog)
	}

	return len(b), nil
}

func newPluginLogForwarder(w io.Writer) *slog.Logger {
	dropForwardedPrefixes := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.SourceKey {
			return slog.Attr{}
		}
		return a
	}

	options := &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelDebug,
		ReplaceAttr: dropForwardedPrefixes,
	}

	return slog.New(newProtoHandler(w, options))
}
