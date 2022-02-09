package client

import (
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
)

// Log is a shared logger that can be used in all opnictl commands.
var Log *zap.SugaredLogger

func init() {
	cfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.DebugLevel),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: true,
		Sampling:          nil,
		Encoding:          "console",
		EncoderConfig:     util.EncoderConfig,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
	}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	Log = logger.Sugar()
}
