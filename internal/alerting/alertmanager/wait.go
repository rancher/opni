package alertmanager_internal

import (
	"context"
	"os"
	"time"
)

func waitForAlertmanagerFile(ctx context.Context, configFile string) bool {
	ctxTimeout := 2 * time.Minute
	ctxCa, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()
	for {
		select {
		case <-ctxCa.Done():
			return false
		default:
			_, err := os.Stat(configFile)
			if err == nil {
				return true
			}
		}
	}
	return false
}
