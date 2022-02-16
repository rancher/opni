package logger

import (
	"fmt"
	"io"
	"log"

	"github.com/hashicorp/go-hclog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type hclogAdapter struct {
	logger ExtendedSugaredLogger
}

var _ hclog.Logger = (*hclogAdapter)(nil)

func (la *hclogAdapter) Log(level hclog.Level, msg string, args ...interface{}) {
	switch level {
	case hclog.Trace:
		la.Debug(msg, args...)
	case hclog.Debug:
		la.Debug(msg, args...)
	case hclog.Info:
		la.Info(msg, args...)
	case hclog.Warn:
		la.Warn(msg, args...)
	case hclog.Error:
		la.Error(msg, args...)
	case hclog.NoLevel:
		if hclog.DefaultLevel != hclog.NoLevel {
			la.Log(hclog.DefaultLevel, msg, args...)
		}
	case hclog.Off:
	}
}

func (la *hclogAdapter) Trace(msg string, args ...interface{}) {
	la.logger.Debugw(msg, stripTimestamp(args...)...)
}

func (la *hclogAdapter) Debug(msg string, args ...interface{}) {
	la.logger.Debugw(msg, stripTimestamp(args...)...)
}

func (la *hclogAdapter) Info(msg string, args ...interface{}) {
	la.logger.Infow(msg, stripTimestamp(args...)...)
}

func (la *hclogAdapter) Warn(msg string, args ...interface{}) {
	la.logger.Warnw(msg, stripTimestamp(args...)...)
}

func (la *hclogAdapter) Error(msg string, args ...interface{}) {
	la.logger.Errorw(msg, stripTimestamp(args...)...)
}

func (la *hclogAdapter) IsTrace() bool {
	return la.logger.AtomicLevel().Enabled(zap.DebugLevel)
}

func (la *hclogAdapter) IsDebug() bool {
	return la.logger.AtomicLevel().Enabled(zap.DebugLevel)
}

func (la *hclogAdapter) IsInfo() bool {
	return la.logger.AtomicLevel().Enabled(zap.InfoLevel)
}

func (la *hclogAdapter) IsWarn() bool {
	return la.logger.AtomicLevel().Enabled(zap.WarnLevel)
}

func (la *hclogAdapter) IsError() bool {
	return la.logger.AtomicLevel().Enabled(zap.ErrorLevel)
}

func (la *hclogAdapter) ImpliedArgs() []interface{} {
	return []interface{}{}
}

func (la *hclogAdapter) With(args ...interface{}) hclog.Logger {
	return &hclogAdapter{
		logger: la.logger.XWith(stripTimestamp(args...)...),
	}
}

func (la *hclogAdapter) Name() string {
	return la.logger.Desugar().Check(zapcore.InfoLevel, "").LoggerName
}

func (la *hclogAdapter) Named(name string) hclog.Logger {
	return &hclogAdapter{
		logger: la.logger.XNamed(name),
	}
}

func (la *hclogAdapter) SetLevel(level hclog.Level) {
	la.logger.AtomicLevel().SetLevel(translateLevel(level))
}

func (la *hclogAdapter) ResetNamed(name string) hclog.Logger {
	return la.Named(name) // todo: does zap support this?
}

func (la *hclogAdapter) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	return hclog.Default().StandardLogger(opts)
}

func (la *hclogAdapter) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return hclog.Default().StandardWriter(opts)
}

func NewHCLogger(logger ExtendedSugaredLogger) hclog.Logger {
	return &hclogAdapter{
		logger: &extendedSugaredLogger{
			SugaredLogger: zap.New(logger.Desugar().Core(), zap.AddCallerSkip(1)).Sugar(),
			level:         logger.AtomicLevel(),
		},
	}
}

func translateLevel(level hclog.Level) zapcore.Level {
	switch level {
	case hclog.Trace, hclog.Debug:
		return zap.DebugLevel
	case hclog.Info:
		return zap.InfoLevel
	case hclog.Warn:
		return zap.WarnLevel
	case hclog.Error:
		return zap.ErrorLevel
	case hclog.NoLevel:
		if hclog.DefaultLevel == hclog.NoLevel {
			panic("default level is invalid")
		}
		return translateLevel(hclog.DefaultLevel)
	case hclog.Off:
		return zap.PanicLevel
	default:
		panic(fmt.Sprintf("unknown level: %d", level))
	}
}

func stripTimestamp(args ...interface{}) []interface{} {
	argsCopy := make([]interface{}, 0, len(args))
	// args alternate keys and values, if any key is "timestamp" then skip it
	for i := 0; i < len(args); i += 2 {
		if args[i] != "timestamp" {
			argsCopy = append(argsCopy, args[i], args[i+1])
		}
	}
	return argsCopy
}
