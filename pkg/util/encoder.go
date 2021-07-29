package util

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/ginkgo"
	"github.com/ttacon/chalk"
	"go.uber.org/atomic"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var startTime = atomic.NewInt64(time.Now().Unix())

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
		enc.AppendString(chalk.Dim.TextStyle(name))
	},
	EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(fmt.Sprintf("[%04d]", t.Unix()-startTime.Load()))
	},
	EncodeDuration: zapcore.SecondsDurationEncoder,
}

func NewTestLogger() logr.Logger {
	return zap.New(
		zap.WriteTo(ginkgo.GinkgoWriter),
		zap.UseDevMode(true),
		zap.Encoder(zapcore.NewConsoleEncoder(EncoderConfig)),
	)
}
