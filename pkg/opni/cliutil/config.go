package cliutil

import (
	"errors"
	"fmt"
	"os"

	"log/slog"

	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/logger"
)

func LoadConfigObjectsOrDie(
	configLocation string,
	lg *slog.Logger,
) meta.ObjectList {
	if configLocation == "" {
		// find config file
		path, err := config.FindConfig()
		if err != nil {
			if errors.Is(err, config.ErrConfigNotFound) {
				wd, _ := os.Getwd()
				panic(fmt.Sprintf(`could not find a config file in ["%s","/etc/opni"], and --config was not given`, wd))
			}
			lg.With(
				logger.Err(err),
			).Error("an error occurred while searching for a config file")
			panic("an error occurred while searching for a config file")
		}
		lg.With(
			"path", path,
		).Info("using config file")
		configLocation = path
	}
	objects, err := config.LoadObjectsFromFile(configLocation)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to load config")
		panic("failed to load config")
	}
	return objects
}
