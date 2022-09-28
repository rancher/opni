package backend

import (
	"context"
	"fmt"
	"strings"

	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/pkg/errors"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *LoggingBackend) CanInstall(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	b.WaitForInit()

	namespaceName := b.Namespace
	if namespaceName == "" {
		namespaceName = "default"
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	err := b.K8sClient.Create(ctx, namespace)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.ErrCreateNamespaceFailed(err)
	}

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	if err := b.K8sClient.Get(ctx, types.NamespacedName{
		Name:      b.OpensearchCluster.Name,
		Namespace: b.OpensearchCluster.Namespace,
	}, opensearchCluster); err != nil {
		return nil, errors.ErrCheckOpensearchClusterFailed(err)
	}

	return &emptypb.Empty{}, nil
}

func (b *LoggingBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	b.WaitForInit()

	labels := map[string]string{
		resources.OpniClusterID: req.Cluster.Id,
	}
	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := b.K8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(b.Namespace),
		client.MatchingLabels{resources.OpniClusterID: req.Cluster.Id},
	); err != nil {
		return nil, errors.ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) > 0 {
		return nil, errors.ErrCreateFailedAlreadyExists(req.Cluster.Id)
	}

	// Generate credentials
	userSuffix := string(util.GenerateRandomString(6))

	password := string(util.GenerateRandomString(20))

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("index-%s", strings.ToLower(userSuffix)),
			Namespace: b.Namespace,
			Labels:    labels,
		},
		StringData: map[string]string{
			"password": password,
		},
	}

	if err := b.K8sClient.Create(ctx, userSecret); err != nil {
		return nil, errors.ErrStoreUserCredentialsFailed(err)
	}

	loggingCluster := &opnicorev1beta1.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "logging-",
			Namespace:    b.Namespace,
			Labels:       labels,
		},
		Spec: opnicorev1beta1.LoggingClusterSpec{
			IndexUserSecret: &corev1.LocalObjectReference{
				Name: userSecret.Name,
			},
			OpensearchClusterRef: b.OpensearchCluster,
		},
	}

	if err := b.K8sClient.Create(ctx, loggingCluster); err != nil {
		return nil, errors.ErrStoreClusterFailed(err)
	}

	_, err := b.StorageBackend.UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)),
	)
	if err != nil {
		return nil, err
	}

	b.requestNodeSync(ctx, req.Cluster)

	return nil, nil
}

func (b *LoggingBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: fmt.Sprintf(`opni bootstrap logging {{ arg "input" "Opensearch Cluster Name" "+required" "+default:%s" }} `, b.OpensearchCluster.Name) +
			`{{ arg "select" "Kubernetes Provider" "" "rke" "rke2" "k3s" "aks" "eks" "gke" "+omitEmpty" "+format:--provider={{ value }}" }} ` +
			`--token={{ .Token }} --pin={{ .Pin }} ` +
			`--gateway-url={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }}`,
	}, nil
}
