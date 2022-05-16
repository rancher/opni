package capabilities

import (
	"context"
	"errors"

	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"google.golang.org/protobuf/types/known/emptypb"
)

var ErrUnknownCapability = errors.New("unknown capability")

func Has[T core.MetadataAccessor[U], U core.Capability[U]](
	accessor T,
	capability U,
) bool {
	for _, cap := range accessor.GetCapabilities() {
		if cap.Equal(capability) {
			return true
		}
	}
	return false
}

type Installer interface {
	CanInstall(capabilities ...string) error
	InstallCapabilities(target *core.Reference, capabilities ...string)
	UninstallCapabilities(target *core.Reference, capabilities ...string)
}

type capabilityBackend struct {
	client capability.BackendClient
}

func (cb *capabilityBackend) CanInstall() error {
	_, err := cb.client.CanInstall(context.Background(), &emptypb.Empty{})
	return err
}

func (cb *capabilityBackend) Install(cluster *core.Reference) error {
	_, err := cb.client.Install(context.Background(), &capability.InstallRequest{
		Cluster: cluster,
	})
	return err
}

func (cb *capabilityBackend) Uninstall(cluster *core.Reference) error {
	_, err := cb.client.Uninstall(context.Background(), &capability.UninstallRequest{
		Cluster: cluster,
	})
	return err
}

func (cb *capabilityBackend) InstallerTemplate() string {
	resp, err := cb.client.InstallerTemplate(context.Background(), &emptypb.Empty{})
	if err != nil {
		return "(error)"
	}
	return resp.Template
}

func NewBackend(client capability.BackendClient) capability.Backend {
	return &capabilityBackend{
		client: client,
	}
}
