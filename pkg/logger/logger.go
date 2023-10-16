package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/slogr"
	"github.com/kralicky/gpkg/sync"
	slogmulti "github.com/samber/slog-multi"
	slogsampling "github.com/samber/slog-sampling"
	"github.com/spf13/afero"
)

var (
	asciiLogo = `                     _
  ____  ____  ____  (_)
 / __ \/ __ \/ __ \/ /
/ /_/ / /_/ / / / / /
\____/ .___/_/ /_/_/
    /_/
 Observability + AIOps for Kubernetes
`
	DefaultLogLevel   = slog.LevelDebug
	DefaultWriter     io.Writer
	DefaultAddSource  = true
	pluginGroupPrefix = "plugin"
	NoRepeatInterval  = 3600 * time.Hour // arbitrarily long time to denote one-time sampling
	logFs             afero.Fs
	logFileName       = "opni-logs"
	DefaultTimeFormat = "2006 Jan 02 15:04:05"
	errKey            = "err"
)

var logSampler = &sampler{}

func init() {
	logFs = afero.NewMemMapFs()
}

func AsciiLogo() string {
	return asciiLogo
}

type LoggerOptions struct {
	Level              slog.Level
	AddSource          bool
	ReplaceAttr        func(groups []string, a slog.Attr) slog.Attr
	Writer             io.Writer
	ColorEnabled       bool
	TimeFormat         string
	TotemFormatEnabled bool
	Sampling           *slogsampling.ThresholdSamplingOption
	OmitLoggerName     bool
	FileWriter         io.Writer
}

func ParseLevel(lvl string) slog.Level {
	l := &slog.LevelVar{}
	l.UnmarshalText([]byte(lvl))
	return l.Level()
}

func Err(e error) slog.Attr {
	if e != nil {
		e = noAllocErr{e}
	}
	return slog.Any(errKey, e)
}

type LoggerOption func(*LoggerOptions)

func (o *LoggerOptions) apply(opts ...LoggerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLogLevel(l slog.Level) LoggerOption {
	return func(o *LoggerOptions) {
		o.Level = slog.Level(l)
	}
}

func WithWriter(w io.Writer) LoggerOption {
	return func(o *LoggerOptions) {
		o.Writer = w
	}
}

func WithFileWriter(w io.Writer) LoggerOption {
	return func(o *LoggerOptions) {
		o.FileWriter = w
	}
}

func WithColor(color bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.ColorEnabled = color
	}
}

func WithDisableCaller() LoggerOption {
	return func(o *LoggerOptions) {
		o.AddSource = false
	}
}

func WithTimeFormat(format string) LoggerOption {
	return func(o *LoggerOptions) {
		o.TimeFormat = format
	}
}

func WithSampling(cfg *slogsampling.ThresholdSamplingOption) LoggerOption {
	return func(o *LoggerOptions) {
		o.Sampling = &slogsampling.ThresholdSamplingOption{
			Tick:      cfg.Tick,
			Threshold: cfg.Threshold,
			Rate:      cfg.Rate,
			OnDropped: logSampler.onDroppedHook,
		}
		o.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.MessageKey {
				msg := a.Value.String()
				count, _ := logSampler.dropped.Load(msg)
				if count > 0 {
					numDropped, _ := logSampler.dropped.LoadAndDelete(msg)
					a.Value = slog.StringValue(fmt.Sprintf("x%d %s", numDropped+1, msg))
				}
			}
			return a
		}
	}
}

func WithTotemFormat(enable bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.TotemFormatEnabled = enable
	}
}

func WithOmitLoggerName() LoggerOption {
	return func(o *LoggerOptions) {
		o.OmitLoggerName = true
	}
}

func colorHandlerWithOptions(opts ...LoggerOption) slog.Handler {
	options := &LoggerOptions{
		Writer:         DefaultWriter,
		ColorEnabled:   ColorEnabled(),
		Level:          DefaultLogLevel,
		AddSource:      DefaultAddSource,
		TimeFormat:     DefaultTimeFormat,
		OmitLoggerName: true,
	}

	options.apply(opts...)

	if DefaultWriter == nil {
		DefaultWriter = os.Stderr
	}

	var middlewares []slogmulti.Middleware
	if options.TotemFormatEnabled {
		options.OmitLoggerName = true
		middlewares = append(middlewares, newTotemNameMiddleware())
	}
	if options.Sampling != nil {
		middlewares = append(middlewares, options.Sampling.NewMiddleware())
	}
	var chain *slogmulti.PipeBuilder
	for i, middleware := range middlewares {
		if i == 0 {
			chain = slogmulti.Pipe(middleware)
		} else {
			chain = chain.Pipe(middleware)
		}
	}

	handler := newColorHandler(options.Writer, options)

	if chain != nil {
		handler = chain.Handler(handler)
	}

	if options.FileWriter != nil {
		logFileHandler := newProtoHandler(options.FileWriter, ConfigureProtoOptions(options))

		// distribute logs to handlers in parallel
		return slogmulti.Fanout(handler, logFileHandler)
	}

	return handler
}

func ConfigureProtoOptions(opts *LoggerOptions) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
	}
}

func New(opts ...LoggerOption) *slog.Logger {
	return slog.New(colorHandlerWithOptions(opts...))
}

func NewLogr(opts ...LoggerOption) logr.Logger {
	return slogr.NewLogr(colorHandlerWithOptions(opts...))
}

func NewNop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

func NewPluginLogger(opts ...LoggerOption) *slog.Logger {
	opts = append(opts, WithFileWriter(sharedPluginWriter))

	return New(opts...).WithGroup(pluginGroupPrefix)
}

type sampler struct {
	dropped sync.Map[string, uint64]
}

func (s *sampler) onDroppedHook(_ context.Context, r slog.Record) {
	key := r.Message
	count, _ := s.dropped.LoadOrStore(key, 0)
	s.dropped.Store(key, count+1)
}

func ReadOnlyFile(clusterID string) afero.File {
	f, err := logFs.OpenFile(logFileName+clusterID, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	return f
}

func WriteOnlyFile(clusterID string) afero.File {
	f, err := logFs.OpenFile(logFileName+clusterID, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	return f
}
