package logger

import (
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type fasthttplogAdapter struct {
	logger SugaredLogger
}

var _ fasthttp.Logger = (*fasthttplogAdapter)(nil)

func (la *fasthttplogAdapter) Printf(format string, args ...interface{}) {
	la.logger.Infof(format, args...)
}

func NewFasthttpLogger(logger SugaredLogger) fasthttp.Logger {
	return &fasthttplogAdapter{
		logger: zap.New(logger.Desugar().Core(), zap.AddCallerSkip(1)).Sugar(),
	}
}
