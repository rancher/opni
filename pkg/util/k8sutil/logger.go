package k8sutil

import (
	"log/slog"

	"github.com/go-logr/logr"
	"github.com/rancher/opni/pkg/logger"
)

func NewControllerRuntimeLogger(level slog.Level) logr.Logger {
	return logger.NewLogr(logger.WithTimeFormat("[15:04:05]"))
}
