package noop

import (
	"context"
	"encoding/hex"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/urn"
)

const (
	updateStrategy = "noop"
)

type agentSyncHandler struct{}

var _ update.SyncHandler = (*agentSyncHandler)(nil)

type pluginSyncHandler struct{}

var _ update.SyncHandler = (*pluginSyncHandler)(nil)

func NewPluginSyncHandler() update.SyncHandler {
	return &pluginSyncHandler{}
}

func NewAgentSyncHandler() update.SyncHandler {
	return &agentSyncHandler{}
}

func (n *pluginSyncHandler) Strategy() string {
	return updateStrategy
}

func (n *agentSyncHandler) Strategy() string {
	return updateStrategy
}

var emptyDigest = hex.EncodeToString(make([]byte, 32))

func (n *pluginSyncHandler) GetCurrentManifest(_ context.Context) (*controlv1.UpdateManifest, error) {
	return &controlv1.UpdateManifest{
		Items: []*controlv1.UpdateManifestEntry{
			{
				Package: urn.NewOpniURN(urn.Plugin, updateStrategy, "unmanaged").String(),
				Path:    "unmanaged",
				Digest:  emptyDigest,
			},
		},
	}, nil
}

func (n *agentSyncHandler) GetCurrentManifest(_ context.Context) (*controlv1.UpdateManifest, error) {
	return &controlv1.UpdateManifest{
		Items: []*controlv1.UpdateManifestEntry{
			{
				Package: urn.NewOpniURN(urn.Agent, updateStrategy, "unmanaged").String(),
				Path:    "unmanaged",
				Digest:  emptyDigest,
			},
		},
	}, nil
}

func (*pluginSyncHandler) HandleSyncResults(_ context.Context, _ *controlv1.SyncResults) error {
	return nil
}

func (*agentSyncHandler) HandleSyncResults(_ context.Context, _ *controlv1.SyncResults) error {
	return nil
}

func init() {
	update.RegisterAgentSyncHandlerBuilder(updateStrategy, func(args ...any) (update.SyncHandler, error) {
		return NewAgentSyncHandler(), nil
	})
	update.RegisterPluginSyncHandlerBuilder(updateStrategy, func(args ...any) (update.SyncHandler, error) {
		return NewPluginSyncHandler(), nil
	})
}
