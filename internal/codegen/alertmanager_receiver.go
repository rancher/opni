package codegen

import (
	"reflect"

	amcfg "github.com/prometheus/alertmanager/config"
)

func GenAlertManagerReceiver() error {
	if err := generate[amcfg.Receiver](
		"github.com/rancher/opni/internal/alertmanager/receiver.proto",
		func(rf reflect.StructField) bool {
			return false
		},
	); err != nil {
		return err
	}
	return nil
}
