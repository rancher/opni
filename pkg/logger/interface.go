package logger

import "go.uber.org/zap"

type SugaredLogger interface {
	Desugar() *zap.Logger
	Named(name string) *zap.SugaredLogger
	With(args ...interface{}) *zap.SugaredLogger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	DPanic(args ...interface{})
	Panic(args ...interface{})
	Fatal(args ...interface{})
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	DPanicf(template string, args ...interface{})
	Panicf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})
	Debugw(msg string, keysAndValues ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
	Errorw(msg string, keysAndValues ...interface{})
	DPanicw(msg string, keysAndValues ...interface{})
	Panicw(msg string, keysAndValues ...interface{})
	Fatalw(msg string, keysAndValues ...interface{})
	Sync() error
}

type ExtendedSugaredLogger interface {
	SugaredLogger
	Zap() *zap.SugaredLogger
	AtomicLevel() zap.AtomicLevel
	XWith(args ...interface{}) ExtendedSugaredLogger
	XNamed(name string) ExtendedSugaredLogger
}
