package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
)

func (p *Plugin) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	labels := map[string]string{
		resources.OpniClusterID: req.Cluster.Id,
	}
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := p.k8sClient.List(
		p.ctx,
		loggingClusterList,
		client.InNamespace(p.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: req.Cluster.Id},
	); err != nil {
		return nil, ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) > 0 {
		return nil, ErrCreateFailedAlreadyExists(req.Cluster.Id)
	}

	// Generate credentials
	userSuffix := string(util.GenerateRandomString(6))

	password := string(util.GenerateRandomString(20))

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("index-%s", strings.ToLower(userSuffix)),
			Namespace: p.storageNamespace,
			Labels:    labels,
		},
		StringData: map[string]string{
			"password": password,
		},
	}

	if err := p.k8sClient.Create(p.ctx, userSecret); err != nil {
		return nil, ErrStoreUserCredentialsFailed(err)
	}

	loggingCluster := &opnicorev1beta1.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "logging-",
			Namespace:    p.storageNamespace,
			Labels:       labels,
		},
		Spec: opnicorev1beta1.LoggingClusterSpec{
			IndexUserSecret: &corev1.LocalObjectReference{
				Name: userSecret.Name,
			},
			OpensearchClusterRef: p.opensearchCluster,
		},
	}

	if err := p.k8sClient.Create(p.ctx, loggingCluster); err != nil {
		return nil, ErrStoreClusterFailed(err)
	}

	_, err := p.storageBackend.Get().UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)),
	)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (p *Plugin) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}

func (p *Plugin) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	cluster, err := p.mgmtApi.Get().GetCluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	defaultOpts := capabilityv1.DefaultUninstallOptions{}
	if strings.TrimSpace(req.Options) != "" {
		if err := json.Unmarshal([]byte(req.Options), &defaultOpts); err != nil {
			return nil, fmt.Errorf("failed to unmarshal options: %v", err)
		}
	}

	exists := false
	for _, cap := range cluster.GetMetadata().GetCapabilities() {
		if cap.Name != wellknown.CapabilityLogs {
			continue
		}
		exists = true

		// check for a previous stale task that may not have been cleaned up
		if cap.DeletionTimestamp != nil {
			// if the deletion timestamp is set and the task is not completed, error
			stat, err := p.uninstallController.Get().TaskStatus(cluster.Id)
			if err != nil {
				if util.StatusCode(err) != codes.NotFound {
					return nil, status.Errorf(codes.Internal, "failed to get task status: %v", err)
				}
				// not found, ok to reset
			}
			switch stat.GetState() {
			case task.StateCanceled, task.StateFailed:
				// stale/completed, ok to reset
			case task.StateCompleted:
				// this probably shouldn't happen, but reset anyway to get back to a good state
				return nil, status.Errorf(codes.FailedPrecondition, "uninstall already completed")
			default:
				return nil, status.Errorf(codes.FailedPrecondition, "uninstall is already in progress")
			}
		}
		break
	}

	if !exists {
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the reuqested capability")
	}

	now := timestamppb.Now()
	_, err = p.storageBackend.Get().UpdateCluster(ctx, cluster.Reference(), func(c *opnicorev1.Cluster) {
		for _, cap := range c.Metadata.Capabilities {
			if cap.Name == wellknown.CapabilityLogs {
				cap.DeletionTimestamp = now
				break
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update cluster metadata: %v", err)
	}

	md := uninstall.TimestampedMetadata{
		DefaultUninstallOptions: defaultOpts,
		DeletionTimestamp:       now.AsTime(),
	}
	err = p.uninstallController.Get().LaunchTask(req.Cluster.Id, task.WithMetadata(md))
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (p *Plugin) UninstallStatus(ctx context.Context, cluster *opnicorev1.Reference) (*opnicorev1.TaskStatus, error) {
	return p.uninstallController.Get().TaskStatus(cluster.Id)
}

func (p *Plugin) CancelUninstall(ctx context.Context, cluster *opnicorev1.Reference) (*emptypb.Empty, error) {
	p.uninstallController.Get().CancelTask(cluster.Id)
	return &emptypb.Empty{}, nil
}

func (p *Plugin) Info(context.Context, *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityLogs,
	}, nil
}

func (p *Plugin) CanInstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	namespaceName := p.PluginOptions.storageNamespace
	if namespaceName == "" {
		namespaceName = "default"
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	err := p.k8sClient.Create(p.ctx, namespace)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, ErrCreateNamespaceFailed(err)
	}

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	if err := p.k8sClient.Get(p.ctx, types.NamespacedName{
		Name:      p.opensearchCluster.Name,
		Namespace: p.opensearchCluster.Namespace,
	}, opensearchCluster); err != nil {
		return nil, ErrCheckOpensearchClusterFailed(err)
	}

	//Finally Check that we have an opensearch client for uninstalls
	if !p.opensearchClient.IsSet() {
		return nil, ErrNoOpensearchClient
	}

	return &emptypb.Empty{}, nil
}

func (p *Plugin) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: fmt.Sprintf(`opni bootstrap logging {{ arg "input" "Opensearch Cluster Name" "+required" "+default:%s" }} `, p.opensearchCluster.Name) +
			`{{ arg "select" "Kubernetes Provider" "" "rke" "rke2" "k3s" "aks" "eks" "gke" "+omitEmpty" "+format:--provider={{ value }}" }} ` +
			`--token={{ .Token }} --pin={{ .Pin }} ` +
			`--gateway-url={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }}`,
	}, nil
}
