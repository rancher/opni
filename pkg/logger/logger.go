package logger

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
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
		"\x1b[33mMulti-(Cluster|Tenant) Monitoring\x1b[0m for Kubernetes\n"
	asciiLogo = "                     _         \n" +
		"  ____  ____  ____  (_)___ ___ \n" +
		" / __ \\/ __ \\/ __ \\/ / __ `__ \\\n" +
		"/ /_/ / /_/ / / / / / / / / / /\n" +
		"\\____/ .___/_/ /_/_/_/ /_/ /_/ \n" +
		"    /_/                        \n" +
		"Multi-(Cluster|Tenant) Monitoring for Kubernetes\n"
)

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
		level:         l.level,
	}
}

func (l *extendedSugaredLogger) XNamed(name string) ExtendedSugaredLogger {
	return &extendedSugaredLogger{
		SugaredLogger: l.Named(name),
		level:         l.level,
	}
}

type LoggerOptions struct {
	logLevel zapcore.Level
	writer   io.Writer
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

func New(opts ...LoggerOption) ExtendedSugaredLogger {
	options := &LoggerOptions{
		logLevel: zap.DebugLevel,
	}
	if inTest {
		options.writer = ginkgo.GinkgoWriter
	}
	options.Apply(opts...)
	encoderConfig := zapcore.EncoderConfig{
		MessageKey:    "M",
		LevelKey:      "L",
		TimeKey:       "T",
		NameKey:       "N",
		CallerKey:     "C",
		FunctionKey:   "",
		StacktraceKey: "S",
		LineEnding:    "\n",
		EncodeLevel: func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			if ColorEnabled() {
				zapcore.CapitalColorLevelEncoder(l, enc)
			} else {
				zapcore.CapitalLevelEncoder(l, enc)
			}
		},
		EncodeCaller: func(ec zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(TextStyle(ec.TrimmedPath(), chalk.Dim))
		},
		EncodeName: func(s string, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(Color(s, chalk.Green))
		},
		EncodeDuration:   zapcore.SecondsDurationEncoder,
		EncodeTime:       zapcore.TimeEncoderOfLayout("Jan 02 15:04:05"),
		ConsoleSeparator: " ",
	}
	level := zap.NewAtomicLevelAt(options.logLevel)
	if options.writer != nil {
		ws := zapcore.Lock(zapcore.AddSync(options.writer))
		encoder := zapcore.NewConsoleEncoder(encoderConfig)
		core := zapcore.NewCore(encoder, ws, level)
		return &extendedSugaredLogger{
			SugaredLogger: zap.New(core).Sugar(),
			level:         level,
		}
	}
	zapConfig := zap.Config{
		Level:             level,
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		Sampling:          nil,
		Encoding:          "console",
		EncoderConfig:     encoderConfig,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stdout"},
	}
	lg, err := zapConfig.Build()
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

func ConfigureApp(app *fiber.App, lg SugaredLogger) {
	app.Server().Logger = NewFasthttpLogger(lg)
}
