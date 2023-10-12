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
	DefaultTimeFormat = "2006 Jan 02 15:04:05"
	errKey            = "err"
)

var logSampler = &sampler{}

func AsciiLogo() string {
	return asciiLogo
}

type LoggerOptions struct {
	Level        slog.Level
	AddSource    bool
	ReplaceAttr  func(groups []string, a slog.Attr) slog.Attr
	Writer       io.Writer
	ColorEnabled bool
	Sampling     *slogsampling.ThresholdSamplingOption
	TimeFormat   string
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
			Tick:       cfg.Tick,
			Threshold:  cfg.Threshold,
			Rate:       cfg.Rate,
			OnDropped:  logSampler.onDroppedHook,
			OnAccepted: logSampler.onAcceptedHook,
		}
	}
}

func colorHandlerWithOptions(opts ...LoggerOption) slog.Handler {
	options := &LoggerOptions{
		Writer:       DefaultWriter,
		ColorEnabled: ColorEnabled(),
		Level:        DefaultLogLevel,
		AddSource:    DefaultAddSource,
		TimeFormat:   DefaultTimeFormat,
	}

	options.apply(opts...)

	if DefaultWriter == nil {
		DefaultWriter = os.Stdout
	}

	handler := newColorHandler(options.Writer, options)

	if options.Sampling != nil {
		return slogmulti.
			Pipe(options.Sampling.NewMiddleware()).
			Handler(handler)
	}
	return handler
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

func (s *sampler) onAcceptedHook(_ context.Context, r slog.Record) {
	attrs := []slog.Attr{}

	msg := r.Message
	count, _ := s.dropped.Load(msg)
	if count > 0 {
		numDropped, _ := s.dropped.LoadAndDelete(msg)
		msg = fmt.Sprintf("x%d %s", numDropped+1, msg)
	}

	r.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	r = slog.NewRecord(r.Time, r.Level, msg, r.PC)
	r.AddAttrs(attrs...)
}
