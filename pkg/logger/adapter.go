package logger

import (
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type logAdapter struct {
	logger *zap.SugaredLogger
}

var _ fasthttp.Logger = (*logAdapter)(nil)

func (la *logAdapter) Printf(format string, args ...interface{}) {
	la.logger.Infof(format, args...)
}

func NewFasthttpLogger(logger *zap.SugaredLogger) fasthttp.Logger {
	return &logAdapter{
		logger: logger,
	}
}
