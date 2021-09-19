// Package common contains utilities shared between the opnictl commands
// and the CLI logic that is not tied to any specific command.
package common

import (
	"time"

	"github.com/rancher/opni/pkg/util"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// Log is a shared logger that can be used in all opnictl commands.
var Log *zap.SugaredLogger
var startTime = atomic.NewInt64(time.Now().Unix())

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
