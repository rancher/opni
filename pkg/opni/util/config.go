package cliutil

import (
	"errors"
	"os"

	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"
)

func LoadConfigObjectsOrDie(
	configLocation string,
	lg logger.ExtendedSugaredLogger,
) meta.ObjectList {
	if configLocation == "" {
		// find config file
		path, err := config.FindConfig()
		if err != nil {
			if errors.Is(err, config.ErrConfigNotFound) {
				wd, _ := os.Getwd()
				lg.Fatalf(`could not find a config file in ["%s","/etc/opni"], and --config was not given`, wd)
			}
			lg.With(
				zap.Error(err),
			).Fatal("an error occurred while searching for a config file")
		}
		lg.With(
			"path", path,
		).Info("using config file")
		configLocation = path
	}
	objects, err := config.LoadObjectsFromFile(configLocation)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load config")
	}
	return objects
}
