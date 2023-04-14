package testlog

import "github.com/rancher/opni/pkg/logger"

var Log = logger.New(logger.WithLogLevel(logger.DefaultLogLevel.Level())).Named("test")
