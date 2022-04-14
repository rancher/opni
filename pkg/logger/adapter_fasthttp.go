package logger

import (
	"strings"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type fasthttplogAdapter struct {
	logger SugaredLogger
}

var _ fasthttp.Logger = (*fasthttplogAdapter)(nil)

func (la *fasthttplogAdapter) Printf(format string, args ...interface{}) {
	if strings.Contains(format, "error") || strings.Contains(format, "failed") {
		la.logger.Errorf(format, args...)
	} else {
		la.logger.Infof(format, args...)
	}
}

func NewFasthttpLogger(name string) fasthttp.Logger {
	return &fasthttplogAdapter{
		logger: New(WithZapOptions(zap.AddCallerSkip(1))).Named(name),
	}
}
