package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/slogr"
	"github.com/kralicky/gpkg/sync"
	slogmulti "github.com/samber/slog-multi"
	slogsampling "github.com/samber/slog-sampling"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
	NoRepeatInterval  = 365 * 24 * time.Hour // arbitrarily long time to denote one-time sampling
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
			Tick:      cfg.Tick,
			Threshold: cfg.Threshold,
			Rate:      cfg.Rate,
			OnDropped: logSampler.onDroppedHook,
		}
		o.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.MessageKey {
				msg := a.Value.String()
				count, _ := logSampler.dropped.LoadOrStore(msg, 0)
				if count > 0 {
					numDropped, _ := logSampler.dropped.LoadAndDelete(msg)
					a.Value = slog.StringValue(fmt.Sprintf("x%d %s", numDropped+1, msg))
				}
			}
			return a
		}
	}
}

func New(opts ...LoggerOption) *slog.Logger {
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
		return slog.New(slogmulti.
			Pipe(options.Sampling.NewMiddleware()).
			Handler(handler))
	}

	return slog.New(handler)
}

func NewLogr(opts ...LoggerOption) logr.Logger {
	options := &LoggerOptions{
		Writer:       DefaultWriter,
		ColorEnabled: colorEnabled,
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
		return slogr.NewLogr(slogmulti.
			Pipe(options.Sampling.NewMiddleware()).
			Handler(handler))
	}

	return slogr.NewLogr(handler)
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

// todo: replace remaining zap loggers with slog when their dependencies support slog
func NewZap(opts ...LoggerOption) *zap.SugaredLogger {
	options := &LoggerOptions{
		Level:  DefaultLogLevel,
		Writer: DefaultWriter,
	}

	options.apply(opts...)
	var color bool
	if options.ColorEnabled {
		color = options.ColorEnabled
	} else {
		color = ColorEnabled()
	}
	encoderConfig := zapcore.EncoderConfig{
		MessageKey:    "M",
		LevelKey:      "L",
		TimeKey:       "T",
		NameKey:       "N",
		CallerKey:     "C",
		FunctionKey:   "",
		StacktraceKey: "S",
		LineEnding:    "\n",
		EncodeLevel:   zapcore.CapitalColorLevelEncoder,
		EncodeCaller: func(ec zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			if color {
				enc.AppendString(TextStyle(ec.TrimmedPath(), chalk.Dim))
			} else {
				enc.AppendString(ec.TrimmedPath())
			}
		},
		EncodeName: func(name string, enc zapcore.PrimitiveArrayEncoder) {
			if len(name) == 0 {
				return
			}
			if strings.HasPrefix(name, "plugin.") {
				enc.AppendString(Color(name, chalk.Cyan))
			} else {
				enc.AppendString(Color(name, chalk.Green))
			}
		},
		EncodeDuration:   zapcore.SecondsDurationEncoder,
		EncodeTime:       zapcore.RFC3339TimeEncoder,
		ConsoleSeparator: " ",
	}
	lvlStr, err := zapcore.ParseLevel(options.Level.Level().String())
	if err != nil {
		panic(err)
	}
	level := zap.NewAtomicLevelAt(lvlStr)
	if options.Writer != nil {
		ws := zapcore.Lock(zapcore.AddSync(options.Writer))
		encoder := zapcore.NewConsoleEncoder(encoderConfig)
		core := zapcore.NewCore(encoder, ws, level)
		return zap.New(core).Sugar()
	}
	zapConfig := zap.Config{
		Level:             level,
		Development:       false,
		DisableCaller:     !options.AddSource,
		DisableStacktrace: true,
		Encoding:          "console",
		EncoderConfig:     encoderConfig,
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	lg, err := zapConfig.Build()
	if err != nil {
		panic(err)
	}
	return lg.Sugar()
}
