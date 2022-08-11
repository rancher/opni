package logging

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
)

func (p *Plugin) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*emptypb.Empty, error) {
	labels := map[string]string{
		resources.OpniClusterID: req.Cluster.Id,
	}
	loggingClusterList := &opniv1beta2.LoggingClusterList{}
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

	loggingCluster := &opniv1beta2.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "logging-",
			Namespace:    p.storageNamespace,
			Labels:       labels,
		},
		Spec: opniv1beta2.LoggingClusterSpec{
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

func (p *Plugin) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	cluster := req.Cluster
	var loggingCluster *opniv1beta2.LoggingCluster
	var secret *corev1.Secret

	loggingClusterList := &opniv1beta2.LoggingClusterList{}
	if err := p.k8sClient.List(
		p.ctx,
		loggingClusterList,
		client.InNamespace(p.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: cluster.Id},
	); err != nil {
		return nil, ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) > 1 {
		return nil, ErrDeleteClusterInvalidList(cluster.Id)
	}
	if len(loggingClusterList.Items) == 1 {
		loggingCluster = &loggingClusterList.Items[0]
	}

	secretList := &corev1.SecretList{}
	if err := p.k8sClient.List(
		p.ctx,
		secretList,
		client.InNamespace(p.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: cluster.Id},
	); err != nil {
		return nil, ErrListingClustersFaled(err)
	}

	if len(secretList.Items) > 1 {
		return nil, ErrDeleteClusterInvalidList(cluster.Id)
	}
	if len(secretList.Items) == 1 {
		secret = &secretList.Items[0]
	}

	if loggingCluster != nil {
		if err := p.k8sClient.Delete(p.ctx, loggingCluster); err != nil {
			return nil, err
		}
	}

	if secret != nil {
		if err := p.k8sClient.Delete(p.ctx, secret); err != nil {
			// Try and be transactionally safe so recreate logging cluster
			labels := map[string]string{
				resources.OpniClusterID: cluster.Id,
			}
			loggingClusterCreate := &opniv1beta2.LoggingCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      loggingCluster.Name,
					Namespace: p.storageNamespace,
					Labels:    labels,
				},
				Spec: opniv1beta2.LoggingClusterSpec{
					IndexUserSecret: &corev1.LocalObjectReference{
						Name: secret.Name,
					},
					OpensearchClusterRef: p.opensearchCluster,
				},
			}
			// error doesn't matter at this point
			p.k8sClient.Create(p.ctx, loggingClusterCreate)

			return nil, err
		}
	}

	_, err := p.storageBackend.Get().UpdateCluster(ctx, &opnicorev1.Reference{
		Id: cluster.Id,
	}, storage.NewRemoveCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)))
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (p *Plugin) UninstallStatus(context.Context, *opnicorev1.Reference) (*opnicorev1.TaskStatus, error) {
	return &opnicorev1.TaskStatus{
		State: task.StateCompleted,
	}, nil
}

func (p *Plugin) CancelUninstall(context.Context, *opnicorev1.Reference) (*emptypb.Empty, error) {
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
