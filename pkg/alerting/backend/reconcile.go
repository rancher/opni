package backend

import (
	"fmt"
	cfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/rancher/opni/pkg/alerting/config"
	"go.uber.org/zap"
	"time"
)

// ValidateIncomingConfig
// Adapted from https://github.com/prometheus/alertmanager/blob/c732372d7d3be49198398d34753080459f01749e/cli/check_config.go#L51
func ValidateIncomingConfig(fileContent string, lg *zap.SugaredLogger) error {
	config, err := cfg.Load(fileContent)
	if err != nil {
		return err
	}
	if config != nil {
		if config.Global != nil {
			lg.Debug("Global config found")
		}
		if config.Route != nil {
			lg.Debug("Route config found")
		}
		lg.Debug(fmt.Sprintf(" - %d inhibit rules", len(config.InhibitRules)))
		lg.Debug(fmt.Sprintf(" - %d receivers", len(config.Receivers)))
		lg.Debug(fmt.Sprintf(" - %d templates", len(config.Templates)))

		if len(config.Templates) > 0 {
			_, err = template.FromGlobs(config.Templates...)
			if err != nil {
				lg.Error(fmt.Sprintf("failed to glob template files with %s for content : %s", err, fileContent))
				return err
			}
		}
	}
	return nil
}

// ReconcileInvalidState : tries to fix detected errors in Alertmanager
func ReconcileInvalidState(newConfig *config.ConfigMapData, incoming error) error {
	if incoming == nil {
		return nil
	}
	switch msg := incoming.Error(); {
	case msg == NoSmartHostSet:
		newConfig.SetDefaultSMTPServer()
	case msg == NoSMTPFromSet:
		newConfig.SetDefaultSMTPFrom()
	case FieldNotFound.MatchString(msg):
		panic(fmt.Sprintf("Likely mismatched versions of prometheus/common : %s", msg))
	default:
		return incoming
	}
	return nil
}

func ReconcileInvalidStateLoop(timeout time.Duration, newConfig *config.ConfigMapData, lg *zap.SugaredLogger) error {
	timeoutTicker := time.NewTicker(timeout)
	var lastSetError error
	for {
		select {
		case <-timeoutTicker.C:
			if lastSetError != nil {
				lastSetError = fmt.Errorf(
					"timeout(%s) when reconciling new alert configs : %s", timeout, lastSetError)
			}
			return lastSetError
		default:
			rawConfig, marshalErr := newConfig.Marshal()
			if marshalErr != nil {
				lastSetError = marshalErr
				continue
			}
			reconcileError := ValidateIncomingConfig(string(rawConfig), lg)
			if reconcileError == nil {
				return nil // success
			}
			err := ReconcileInvalidState(newConfig, reconcileError)
			if err != nil {
				lastSetError = err
				continue
			} // can't return nil after this as there may be a chain of errors to handle
		}
	}
}
