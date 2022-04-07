package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/kralicky/gpkg/sync"
	"github.com/onsi/ginkgo/v2"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	coloredAsciiLogo = "                     _         \n" +
		"  ____  ____  ____  (_)\x1b[33m___ ___\x1b[0m\n" +
		" / __ \\/ __ \\/ __ \\/ \x1b[33m/ __ " + "`" + "__ \\\x1b[0m\n" +
		"/ /_/ / /_/ / / / / \x1b[33m/ / / / / /\x1b[0m\n" +
		"\\____/ .___/_/ /_/_\x1b[33m/_/ /_/ /_/\x1b[0m\n" +
		"    /_/                        \n" +
		"\x1b[33mMulti-Cluster Monitoring\x1b[0m for Kubernetes\n"
	asciiLogo = "                     _         \n" +
		"  ____  ____  ____  (_)___ ___ \n" +
		" / __ \\/ __ \\/ __ \\/ / __ `__ \\\n" +
		"/ /_/ / /_/ / / / / / / / / / /\n" +
		"\\____/ .___/_/ /_/_/_/ /_/ /_/ \n" +
		"    /_/                        \n" +
		"Multi-Cluster Monitoring for Kubernetes\n"

	levelToColor = map[zapcore.Level]chalk.Color{
		zapcore.DebugLevel:  chalk.Magenta,
		zapcore.InfoLevel:   chalk.Blue,
		zapcore.WarnLevel:   chalk.Yellow,
		zapcore.ErrorLevel:  chalk.Red,
		zapcore.DPanicLevel: chalk.Red,
		zapcore.PanicLevel:  chalk.Red,
		zapcore.FatalLevel:  chalk.Red,
	}

	levelToColorString = make(map[zapcore.Level]string, len(levelToColor))
)

func init() {
	for level, color := range levelToColor {
		levelToColorString[level] = color.Color(level.CapitalString())
	}
}

func AsciiLogo() string {
	if ColorEnabled() {
		return coloredAsciiLogo
	}
	return asciiLogo
}

type loggerContextKey struct{}

var key = loggerContextKey{}

var inTest = strings.HasSuffix(os.Args[0], ".test")

type extendedSugaredLogger struct {
	SugaredLogger
	level zap.AtomicLevel
}

func (l *extendedSugaredLogger) Zap() *zap.SugaredLogger {
	return l.SugaredLogger.(*zap.SugaredLogger)
}

func (l *extendedSugaredLogger) AtomicLevel() zap.AtomicLevel {
	return l.level
}

func (l *extendedSugaredLogger) XWith(args ...interface{}) ExtendedSugaredLogger {
	return &extendedSugaredLogger{
		SugaredLogger: l.With(args...),
		level:         zap.NewAtomicLevelAt(l.level.Level()),
	}
}

func (l *extendedSugaredLogger) XNamed(name string) ExtendedSugaredLogger {
	return &extendedSugaredLogger{
		SugaredLogger: l.Named(name),
		level:         zap.NewAtomicLevelAt(l.level.Level()),
	}
}

type LoggerOptions struct {
	logLevel   zapcore.Level
	writer     io.Writer
	color      *bool
	zapOptions []zap.Option
	sampling   *zap.SamplingConfig
}

type LoggerOption func(*LoggerOptions)

func (o *LoggerOptions) Apply(opts ...LoggerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLogLevel(l zapcore.Level) LoggerOption {
	return func(o *LoggerOptions) {
		o.logLevel = l
	}
}

func WithWriter(w io.Writer) LoggerOption {
	return func(o *LoggerOptions) {
		o.writer = w
	}
}

func WithColor(color bool) LoggerOption {
	return func(o *LoggerOptions) {
		o.color = &color
	}
}

func WithZapOptions(opts ...zap.Option) LoggerOption {
	return func(o *LoggerOptions) {
		o.zapOptions = append(o.zapOptions, opts...)
	}
}

func WithSampling(cfg *zap.SamplingConfig) LoggerOption {
	return func(o *LoggerOptions) {
		s := &sampler{}
		o.sampling = &zap.SamplingConfig{
			Initial:    cfg.Initial,
			Thereafter: cfg.Thereafter,
			Hook:       s.Hook,
		}
	}
}

func New(opts ...LoggerOption) ExtendedSugaredLogger {
	options := &LoggerOptions{
		logLevel: zap.DebugLevel,
	}
	if inTest {
		options.writer = ginkgo.GinkgoWriter
	}
	options.Apply(opts...)
	var color bool
	if options.color != nil {
		color = *options.color
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
		EncodeName: func(s string, enc zapcore.PrimitiveArrayEncoder) {
			if len(s) == 0 {
				return
			}
			var name string
			if s[0] == 'x' {
				// if the string starts with 'x', check if the name is prefixed with the
				// dropped sample count of the form "x## " and if so, color it differently
			LOOP:
				for i := 1; i < len(s); i++ {
					switch s[i] {
					case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
						continue
					case ' ':
						enc.AppendString(Color(s[:i], chalk.White))
						name = s[i+1:]
						break LOOP
					default:
						name = s
						break LOOP
					}
				}
			} else {
				name = s
			}

			if strings.HasPrefix(name, "plugin.") {
				enc.AppendString(Color(name, chalk.Cyan))
			} else {
				enc.AppendString(Color(name, chalk.Green))
			}
		},
		EncodeDuration:   zapcore.SecondsDurationEncoder,
		EncodeTime:       zapcore.ISO8601TimeEncoder,
		ConsoleSeparator: " ",
	}
	level := zap.NewAtomicLevelAt(options.logLevel)
	if options.writer != nil {
		ws := zapcore.Lock(zapcore.AddSync(options.writer))
		encoder := zapcore.NewConsoleEncoder(encoderConfig)
		core := zapcore.NewCore(encoder, ws, level)
		return &extendedSugaredLogger{
			SugaredLogger: zap.New(core, options.zapOptions...).Sugar(),
			level:         level,
		}
	}
	zapConfig := zap.Config{
		Level:             level,
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		Sampling:          options.sampling,
		Encoding:          "console",
		EncoderConfig:     encoderConfig,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	lg, err := zapConfig.Build(options.zapOptions...)
	if err != nil {
		panic(err)
	}
	return &extendedSugaredLogger{
		SugaredLogger: lg.Sugar(),
		level:         level,
	}
}

func AddToContext(ctx context.Context, lg ExtendedSugaredLogger) context.Context {
	return context.WithValue(ctx, key, lg)
}

func FromContext(ctx context.Context) ExtendedSugaredLogger {
	lg, ok := ctx.Value(key).(ExtendedSugaredLogger)
	if !ok {
		panic("logger not found in context")
	}
	return lg
}

func ConfigureAppLogger(app *fiber.App, name string) {
	app.Server().Logger = NewFasthttpLogger(name)
}

func NewForPlugin() hclog.Logger {
	opts := &hclog.LoggerOptions{
		Level:       hclog.Debug,
		JSONFormat:  true,
		DisableTime: true,
	}
	if inTest {
		opts.Output = ginkgo.GinkgoWriter
	}
	return hclog.New(opts)
}

type sampler struct {
	dropped sync.Map[string, uint64]
}

func (s *sampler) Hook(e *zapcore.Entry, sd zapcore.SamplingDecision) {
	key := e.Message
	count, _ := s.dropped.LoadOrStore(key, 0)
	switch sd {
	case zapcore.LogDropped:
		s.dropped.Store(key, count+1)
	case zapcore.LogSampled:
		if count > 0 {
			numDropped, _ := s.dropped.LoadAndDelete(key)
			e.LoggerName = fmt.Sprintf("x%d %s", numDropped+1, e.LoggerName)
		}
	}
}
