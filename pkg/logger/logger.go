package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kralicky/gpkg/sync"
	"github.com/lmittmann/tint"
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

	levelToColor = map[slog.Level]chalk.Color{
		slog.LevelDebug: chalk.Magenta,
		slog.LevelInfo:  chalk.Blue,
		slog.LevelWarn:  chalk.Yellow,
		slog.LevelError: chalk.Red,
	}

	levelToColorString = make(map[any]string, len(levelToColor))
	DefaultLogLevel    = slog.LevelDebug
	DefaultWriter      io.Writer
	DefaultAddSource   = true
	DefaultDisableTime = false
	pluginGroupPrefix  = "plugin"
	NoRepeatInterval   = 365 * 24 * time.Hour // arbitrarily long time to denote one-time sampling
)

func init() {
	for level, color := range levelToColor {
		levelToColorString[level.String()] = color.Color(level.String())
	}
}

func AsciiLogo() string {
	return asciiLogo
}

var s = &sampler{}

type LoggerOptions struct {
	Level       slog.Level
	AddSource   bool
	DisableTime bool
	Color       *bool
	Writer      io.Writer
	Sampling    *slogsampling.ThresholdSamplingOption
	ReplaceAttr func(groups []string, a slog.Attr) slog.Attr
}

func ParseLevel(lvl string) slog.Level {
	l := &slog.LevelVar{}
	l.UnmarshalText([]byte(lvl))
	return l.Level()
}

func Err(e error) slog.Attr {
	return tint.Err(e)
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
		o.Color = &color
	}
}

func WithDisableCaller() LoggerOption {
	return func(o *LoggerOptions) {
		o.AddSource = false
	}
}

func WithDisableTime() LoggerOption {
	return func(o *LoggerOptions) {
		o.DisableTime = true
	}
}

func WithSampling(cfg *slogsampling.ThresholdSamplingOption) LoggerOption {
	return func(o *LoggerOptions) {
		o.Sampling = &slogsampling.ThresholdSamplingOption{
			Tick:      cfg.Tick,
			Threshold: cfg.Threshold,
			Rate:      cfg.Rate,
			OnDropped: s.onDroppedHook,
		}
	}
}

func logWithDroppedCount(a slog.Attr) slog.Attr {
	key := a.Value.String()
	count, _ := s.dropped.LoadOrStore(key, 0)
	if count > 0 {
		numDropped, _ := s.dropped.LoadAndDelete(key)
		return slog.String(a.Key, fmt.Sprintf("x%d %s", numDropped+1, key))
	}
	return a
}

func trimSourcePath(a slog.Attr) string {
	source := a.Value.Any().(*slog.Source)
	dir, file := filepath.Split(source.File)
	source.File = filepath.Join(filepath.Base(dir), file)
	return fmt.Sprintf("%s:%d", source.File, source.Line)
}

func ConfigureOptions(opts *LoggerOptions) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.MessageKey:
				return logWithDroppedCount(a)
			case slog.SourceKey:
				trimmedPath := trimSourcePath(a)
				groupName := strings.Join(groups, ".")
				return slog.String(a.Key, fmt.Sprintf("%s %s", groupName, trimmedPath))
			default:
				return a
			}
		},
	}
}

func ConfigureColorOptions(opts *LoggerOptions) *tint.Options {
	return &tint.Options{
		Level:     opts.Level,
		AddSource: opts.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.MessageKey:
				logWithDroppedCount(a)
			case slog.LevelKey:
				return slog.String(a.Key, levelToColorString[a.Value.String()])
			case slog.SourceKey:
				trimmedPath := TextStyle(trimSourcePath(a), chalk.Dim)
				groupName := strings.Join(groups, ".")
				if len(groups) > 0 && groups[0] == "plugin" {
					groupName = chalk.Cyan.Color(groupName)
				} else {
					groupName = chalk.Green.Color(groupName)
				}
				return slog.String(a.Key, fmt.Sprintf("%s %s", groupName, trimmedPath))
			}
			return a
		},
	}
}

func New(opts ...LoggerOption) *slog.Logger {
	options := &LoggerOptions{
		Level:       DefaultLogLevel,
		AddSource:   DefaultAddSource,
		DisableTime: DefaultDisableTime,
		Writer:      DefaultWriter,
	}

	options.apply(opts...)

	if DefaultWriter == nil {
		DefaultWriter = os.Stdout
	}

	var color bool
	if options.Color != nil {
		color = *options.Color
	} else {
		color = ColorEnabled()
	}

	var handler slog.Handler
	if !color {
		formatted := ConfigureOptions(options)
		handler = slog.NewTextHandler(options.Writer, formatted)
	} else {
		colorFormatted := ConfigureColorOptions(options)
		handler = tint.NewHandler(options.Writer, colorFormatted)
	}

	if options.Sampling != nil {
		return slog.New(slogmulti.
			Pipe(options.Sampling.NewMiddleware()).
			Handler(handler))
	}

	return slog.New(handler)
}

func NewNop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

func NewPluginLogger() *slog.Logger {
	return New().WithGroup("plugin")
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
	if options.Color != nil {
		color = *options.Color
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
