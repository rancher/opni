package backend

import (
	"context"
	"errors"
	"time"

	"github.com/gogo/protobuf/proto"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *LoggingBackend) Status(ctx context.Context, req *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	b.WaitForInit()

	b.nodeStatusMu.RLock()
	defer b.nodeStatusMu.RUnlock()

	capStatus, err := b.getStatus(ctx, req.GetCluster().GetId())
	if err != nil {
		if errors.Is(err, loggingerrors.ErrInvalidList) {
			return nil, status.Error(codes.NotFound, "unable to list cluster status")
		}
		return nil, err
	}

	return capStatus, nil
}

func (b *LoggingBackend) Sync(ctx context.Context, req *node.SyncRequest) (*node.SyncResponse, error) {
	b.WaitForInit()

	id, ok := cluster.AuthorizedIDFromIncomingContext(ctx)
	if !ok {
		return nil, util.StatusError(codes.Unauthenticated)
	}

	// look up the cluster and check if the capability is installed
	cluster, err := b.StorageBackend.GetCluster(ctx, &opnicorev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, err
	}

	var enabled bool
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityLogs {
			enabled = (cap.DeletionTimestamp == nil)
		}
	}
	var conditions []string
	if enabled {
		if b.shouldDisableNode(ctx) {
			reason := "opensearch is not installed"
			enabled = false
			conditions = append(conditions, reason)
		}
	}

	b.desiredNodeSpecMu.RLock()
	defer b.desiredNodeSpecMu.RUnlock()

	b.nodeStatusMu.RLock()
	defer b.nodeStatusMu.RUnlock()

	err = b.updateClusterStatus(ctx, id, time.Now(), enabled)
	if err != nil {
		return nil, err
	}

	osConf, err := b.getOpensearchConfig(ctx, id)
	if err != nil {
		return nil, err
	}

	return b.buildResponse(req.CurrentConfig, &node.LoggingCapabilityConfig{
		Enabled:          enabled,
		Conditions:       conditions,
		OpensearchConfig: osConf,
	}), nil
}

func (b *LoggingBackend) buildResponse(old, new *node.LoggingCapabilityConfig) *node.SyncResponse {
	if proto.Equal(old, new) {
		return &node.SyncResponse{
			ConfigStatus: node.ConfigStatus_UpToDate,
		}
	}
	return &node.SyncResponse{
		ConfigStatus:  node.ConfigStatus_NeedsUpdate,
		UpdatedConfig: new,
	}
}

func (b *LoggingBackend) getOpensearchConfig(ctx context.Context, id string) (*node.OpensearchConfig, error) {
	opnimgmt := &loggingv1beta1.OpniOpensearch{}
	if err := b.K8sClient.Get(ctx, types.NamespacedName{
		Name:      b.OpensearchCluster.Name,
		Namespace: b.Namespace,
	}, opnimgmt); err != nil {
		b.Logger.Errorf("unable to fetch opniopensearch object: %v", err)
		return nil, err
	}

	labels := map[string]string{
		resources.OpniClusterID: id,
	}
	secrets := &corev1.SecretList{}
	if err := b.K8sClient.List(ctx, secrets, client.InNamespace(b.Namespace), client.MatchingLabels(labels)); err != nil {
		b.Logger.Errorf("unable to list secrets: %v", err)
		return nil, err
	}

	if len(secrets.Items) != 1 {
		b.Logger.Error("no credential secrets found")
		return nil, loggingerrors.ErrGetDetailsInvalidList(id)
	}

	return &node.OpensearchConfig{
		Username: secrets.Items[0].Name,
		Password: string(secrets.Items[0].Data["password"]),
		Url: func() string {
			if opnimgmt != nil {
				return opnimgmt.Spec.ExternalURL
			}
			return ""
		}(),
		TracingEnabled: true,
	}, nil
}

func (b *LoggingBackend) shouldDisableNode(ctx context.Context) bool {
	cluster := &loggingv1beta1.OpniOpensearch{}
	if err := b.K8sClient.Get(ctx, types.NamespacedName{
		Name:      b.OpensearchCluster.Name,
		Namespace: b.OpensearchCluster.Namespace,
	}, cluster); err != nil {
		return k8serrors.IsNotFound(err)
	}
	return false
}

func (b *LoggingBackend) getStatus(ctx context.Context, id string) (*capabilityv1.NodeCapabilityStatus, error) {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := b.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(b.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		return nil, loggingerrors.ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) != 1 {
		return nil, loggingerrors.ErrInvalidList
	}

	return &capabilityv1.NodeCapabilityStatus{
		Enabled:  loggingClusterList.Items[0].Spec.Enabled,
		LastSync: timestamppb.New(loggingClusterList.Items[0].Spec.LastSync.Time),
	}, nil
}

func (b *LoggingBackend) updateClusterStatus(ctx context.Context, id string, syncTime time.Time, enabled bool) error {
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := b.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(b.Namespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		return loggingerrors.ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) != 1 {
		return loggingerrors.ErrInvalidList
	}

	cluster := &loggingClusterList.Items[0]

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := b.K8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
		if err != nil {
			return err
		}
		cluster.Spec.LastSync = metav1.NewTime(syncTime)
		cluster.Spec.Enabled = enabled
		return b.K8sClient.Update(ctx, cluster)
	})
}

func (b *LoggingBackend) requestNodeSync(ctx context.Context, cluster *opnicorev1.Reference) {
	_, err := b.NodeManagerClient.RequestSync(ctx, &capabilityv1.SyncRequest{
		Cluster: cluster,
		Filter: &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityLogs},
		},
	})

	name := cluster.GetId()
	if name == "" {
		name = "(all)"
	}
	if err != nil {
		b.Logger.With(
			"cluster", name,
			"capability", wellknown.CapabilityLogs,
			zap.Error(err),
		).Warn("failed to request node sync; nodes may not be updated immediately")
		return
	}
	b.Logger.With(
		"cluster", name,
		"capability", wellknown.CapabilityLogs,
	).Info("node sync requested")
}
