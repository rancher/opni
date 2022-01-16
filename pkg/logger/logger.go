package logger

import (
	"context"

	"github.com/gofiber/fiber/v2"
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

func New() *zap.SugaredLogger {
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
	zapConfig := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.DebugLevel),
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
	return lg.Sugar()
}

func AddToContext(ctx context.Context, lg *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, key, lg)
}

func FromContext(ctx context.Context) *zap.SugaredLogger {
	lg, ok := ctx.Value(key).(*zap.SugaredLogger)
	if !ok {
		panic("logger not found in context")
	}
	return lg
}

func ConfigureApp(app *fiber.App, lg *zap.SugaredLogger) {
	app.Server().Logger = NewFasthttpLogger(lg)
}
