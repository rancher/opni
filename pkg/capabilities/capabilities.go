package capabilities

import (
	"context"
	"errors"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"google.golang.org/protobuf/types/known/emptypb"
)

var ErrUnknownCapability = errors.New("unknown capability")

func Has[T corev1.MetadataAccessor[U], U corev1.Capability[U]](
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
	InstallCapabilities(target *corev1.Reference, capabilities ...string)
	UninstallCapabilities(target *corev1.Reference, capabilities ...string) error
}

type capabilityBackend struct {
	client capability.BackendClient
}

func (cb *capabilityBackend) CanInstall() error {
	_, err := cb.client.CanInstall(context.Background(), &emptypb.Empty{})
	return err
}

func (cb *capabilityBackend) Install(cluster *corev1.Reference) error {
	_, err := cb.client.Install(context.Background(), &capability.InstallRequest{
		Cluster: cluster,
	})
	return err
}

func (cb *capabilityBackend) Uninstall(cluster *corev1.Reference) error {
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
