package util

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo"
	"github.com/ttacon/chalk"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var EncoderConfig = zapcore.EncoderConfig{
	MessageKey:       "M",
	LevelKey:         "L",
	TimeKey:          "T",
	NameKey:          "N",
	CallerKey:        "C",
	FunctionKey:      "",
	StacktraceKey:    "S",
	ConsoleSeparator: " ",
	EncodeLevel:      zapcore.CapitalColorLevelEncoder,
	EncodeCaller: func(ec zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(chalk.Dim.TextStyle(ec.TrimmedPath()))
	},
	EncodeName: func(name string, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(chalk.Dim.TextStyle(
			strings.Replace(name, "controller-runtime.manager.", "", 1)))
	},
	EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("[15:04:05]"))
	},
	EncodeDuration: zapcore.SecondsDurationEncoder,
}

func NewTestLogger() logr.Logger {
	return zap.New(
		zap.Level(zapcore.InfoLevel),
		zap.WriteTo(ginkgo.GinkgoWriter),
		zap.UseDevMode(false),
		zap.Encoder(zapcore.NewConsoleEncoder(EncoderConfig)),
	)
}
