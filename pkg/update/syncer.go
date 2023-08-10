package update

import (
	"context"
	"fmt"
	"time"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

type SyncConfig struct {
	Client controlv1.UpdateSyncClient
	Syncer SyncHandler
	Logger *zap.SugaredLogger
}

func (conf SyncConfig) DoSync(ctx context.Context) error {
	syncer := conf.Syncer
	initialManifest, err := getManifestWithTimeout(ctx, syncer)
	if err != nil {
		return err
	}
	updateType, err := GetType(initialManifest.GetItems())
	if err != nil {
		return fmt.Errorf("failed to get manifest update type: %w", err)
	}

	lg := conf.Logger.With(
		zap.String("type", string(updateType)),
	)
	lg.With(
		"entries", len(initialManifest.GetItems()),
	).Debug("sending manifest sync request")

	syncResp, err := conf.Client.SyncManifest(metadata.AppendToOutgoingContext(ctx,
		controlv1.UpdateStrategyKeyForType(updateType), syncer.Strategy(),
	), initialManifest)
	if err != nil {
		return fmt.Errorf("failed to sync agent manifest: %w", err)
	}
	lg.Info("received sync response")
	err = syncer.HandleSyncResults(ctx, syncResp)
	if err != nil {
		return fmt.Errorf("failed to handle agent sync results: %w", err)
	}
	lg.With(
		"entries", len(initialManifest.GetItems()),
	).Info("manifest sync complete")
	return nil
}

func (conf SyncConfig) Result(ctx context.Context) (*controlv1.UpdateManifest, error) {
	return getManifestWithTimeout(ctx, conf.Syncer)
}

func getManifestWithTimeout(ctx context.Context, syncer SyncHandler) (*controlv1.UpdateManifest, error) {
	ctx, ca := context.WithTimeout(ctx, 10*time.Second)
	m, err := syncer.GetCurrentManifest(ctx)
	ca()
	if err != nil {
		return nil, fmt.Errorf("failed to get current manifest: %w", err)
	}
	return m, nil
}
