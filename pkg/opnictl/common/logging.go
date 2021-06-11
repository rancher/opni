// Package common contains utilities shared between the opnictl commands
// and the CLI logic that is not tied to any specific command.
package common

import (
	"fmt"
	"time"

	"github.com/ttacon/chalk"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log is a shared logger that can be used in all opnictl commands.
var Log *zap.SugaredLogger
var startTime = atomic.NewInt64(time.Now().Unix())

func init() {
	encoderCfg := zapcore.EncoderConfig{
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
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(fmt.Sprintf("[%04d]", t.Unix()-startTime.Load()))
		},
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}
	cfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.DebugLevel),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		Sampling:          nil,
		Encoding:          "console",
		EncoderConfig:     encoderCfg,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	Log = logger.Sugar()
}
