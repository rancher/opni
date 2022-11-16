package uninstall

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap/zapcore"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	"github.com/rancher/opni/pkg/task"
)

type TimestampedMetadata struct {
	capabilityv1.DefaultUninstallOptions `json:",inline,omitempty"`
	DeletionTimestamp                    time.Time `json:"deletionTimestamp,omitempty"`
}

type DefaultPendingHandler struct{}

func (DefaultPendingHandler) OnTaskPending(ctx context.Context, ti task.ActiveTask) error {
	var md TimestampedMetadata
	ti.LoadTaskMetadata(&md)

	if md.DeleteStoredData {
		ti.AddLogEntry(zapcore.WarnLevel, "Stored data will be deleted")
	} else {
		ti.AddLogEntry(zapcore.InfoLevel, "Stored data will not be deleted")
	}

	if md.InitialDelay > 0 {
		endTime := md.DeletionTimestamp.Add(time.Duration(md.InitialDelay))
		now := time.Now()
		// sleep until endTime or context is cancelled
		if endTime.After(now) {
			var format string
			if md.DeleteStoredData {
				format = "Delaying uninstall and data deletion until %s (%s from now)"
			} else {
				format = "Delaying uninstall until %s (%s from now)"
			}
			ti.AddLogEntry(zapcore.InfoLevel, fmt.Sprintf(format, endTime.Format(time.Stamp), endTime.Sub(now).Round(time.Second)))
			timer := time.NewTimer(endTime.Sub(now))
			defer timer.Stop()
			select {
			case <-ctx.Done():
				ti.AddLogEntry(zapcore.InfoLevel, "Uninstall canceled during delay period; no changes will be made")
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	return nil
}
