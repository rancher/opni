package common

import (
	"strconv"
	"time"

	"github.com/ttacon/chalk"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
			now := t.Unix()
			var timeBuf []byte
			elapsed := now - startTime.Load()
			number := strconv.AppendInt([]byte{}, elapsed, 10)
			switch len(number) {
			case 1:
				timeBuf = []byte{'[', '0', '0', number[0], ']'}
			case 2:
				timeBuf = []byte{'[', '0', number[0], number[1], ']'}
			case 3:
				timeBuf = []byte{'[', number[0], number[1], number[2], ']'}
			default:
				timeBuf = append(append([]byte{'['}, number...), ']')
			}
			enc.AppendByteString(timeBuf)
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
