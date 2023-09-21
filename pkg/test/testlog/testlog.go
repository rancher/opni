package testlog

import (
	"log/slog"

	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

var Log = lo.TernaryF(testruntime.IsTesting, func() *slog.Logger {
	return logger.New(logger.WithLogLevel(logger.DefaultLogLevel.Level()), logger.WithWriter(ginkgo.GinkgoWriter)).WithGroup("test")
}, func() *slog.Logger {
	return logger.New(logger.WithLogLevel(logger.DefaultLogLevel.Level())).WithGroup("test")
})

var ZapLog = lo.TernaryF(testruntime.IsTesting, func() *zap.SugaredLogger {
	return logger.NewZap(logger.WithLogLevel(logger.DefaultLogLevel.Level()), logger.WithWriter(ginkgo.GinkgoWriter)).Named("test")
}, func() *zap.SugaredLogger {
	return logger.NewZap(logger.WithLogLevel(logger.DefaultLogLevel.Level())).Named("test")
})
