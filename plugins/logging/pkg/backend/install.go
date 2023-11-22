package backend

import (
	"context"
	"encoding/base64"
	"errors"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/crypto"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/supportagent"
	driver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (b *LoggingBackend) canInstall(ctx context.Context) error {
	installState := b.ClusterDriver.GetInstallStatus(ctx)
	switch installState {
	case driver.Absent:
		return status.Error(codes.Unavailable, "opensearch cluster is not installed")
	case driver.Pending, driver.Installed:
		return nil
	case driver.Error:
		fallthrough
	default:
		return status.Error(codes.Internal, "unknown opensearch cluster state")
	}
}

func (b *LoggingBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	b.WaitForInit()

	var warningErr error
	if err := b.canInstall(ctx); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		}
		warningErr = err
	}

	cluster, err := b.StorageBackend.GetCluster(ctx, req.GetAgent())
	if err != nil {
		return nil, err
	}

	name := cluster.GetMetadata().GetLabels()[opnicorev1.NameLabel]

	if err := b.ClusterDriver.StoreCluster(ctx, req.GetAgent(), name); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		}
		warningErr = err
	}

	supportLabelValue, ok := cluster.GetMetadata().GetLabels()[opnicorev1.SupportLabel]
	supportUser := ok && supportLabelValue == "true"
	if supportUser {
		p, err := b.generatePassword(ctx, req.GetAgent())
		if err != nil {
			return nil, err
		}
		err = b.ClusterDriver.StoreClusterReadUser(ctx, name, base64.StdEncoding.EncodeToString(p), cluster.GetId())
		if err != nil {
			return nil, err
		}
	}

	_, err = b.StorageBackend.UpdateCluster(ctx, req.Agent,
		storage.NewAddCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)),
	)
	if err != nil {
		return nil, err
	}

	b.requestNodeSync(ctx, req.Agent)

	if warningErr != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Warning,
			Message: warningErr.Error(),
		}, nil
	}

	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

func (b *LoggingBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-agent --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}

func (b *LoggingBackend) generatePassword(ctx context.Context, cluster *opnicorev1.Reference) ([]byte, error) {
	krStore := b.StorageBackend.KeyringStore("gateway", cluster)
	kr, err := krStore.Get(ctx)
	if err != nil {
		return []byte{}, err
	}
	var sharedKeys *keyring.SharedKeys
	ok := kr.Try(func(key *keyring.SharedKeys) {
		sharedKeys = key
	})
	if !ok {
		return []byte{}, errors.New("keyring does not contain shared keys")
	}

	hasher := crypto.NewCShakeHasher(sharedKeys.ServerKey, supportagent.SupportAgentDomain)

	return hasher.Hash(sharedKeys.ClientKey, 32)
}
