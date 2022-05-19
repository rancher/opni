package logging

import (
	"fmt"
	"strings"

	"github.com/rancher/opni/apis/v1beta2"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Plugin) CanInstall() error {
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
		return ErrCreateNamespaceFailed(err)
	}

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	if err := p.k8sClient.Get(p.ctx, types.NamespacedName{
		Name:      p.opensearchCluster.Name,
		Namespace: p.opensearchCluster.Namespace,
	}, opensearchCluster); err != nil {
		return ErrCheckOpensearchClusterFailed(err)
	}

	return nil
}

func (p *Plugin) Install(cluster *core.Reference) error {
	labels := map[string]string{
		resources.OpniClusterID: cluster.Id,
	}
	loggingClusterList := &opniv1beta2.LoggingClusterList{}
	if err := p.k8sClient.List(
		p.ctx,
		loggingClusterList,
		client.InNamespace(p.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: cluster.Id},
	); err != nil {
		return ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) > 0 {
		return ErrCreateFailedAlreadyExists(cluster.Id)
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
		return ErrStoreUserCredentialsFailed(err)
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
		return ErrStoreClusterFailed(err)
	}

	return nil
}

func (p *Plugin) Uninstall(cluster *core.Reference) error {
	var loggingCluster *v1beta2.LoggingCluster
	var secret *corev1.Secret

	loggingClusterList := &opniv1beta2.LoggingClusterList{}
	if err := p.k8sClient.List(
		p.ctx,
		loggingClusterList,
		client.InNamespace(p.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: cluster.Id},
	); err != nil {
		return ErrListingClustersFaled(err)
	}

	if len(loggingClusterList.Items) > 1 {
		return ErrDeleteClusterInvalidList(cluster.Id)
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
		return ErrListingClustersFaled(err)
	}

	if len(secretList.Items) > 1 {
		return ErrDeleteClusterInvalidList(cluster.Id)
	}
	if len(secretList.Items) == 1 {
		secret = &secretList.Items[0]
	}

	if loggingCluster != nil {
		if err := p.k8sClient.Delete(p.ctx, loggingCluster); err != nil {
			return err
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

			return err
		}
	}

	return nil
}

func (p *Plugin) InstallerTemplate() string {
	return fmt.Sprintf(`opnictl bootstrap logging {{ arg "input" "Opensearch Cluster Name" "+required" "+default:%s" }} `, p.opensearchCluster.Name) +
		`{{ arg "select" "Kubernetes Provider" "" "rke" "rke2" "k3s" "aks" "eks" "gke" "+omitEmpty" "+format:--provider={{ value }}" }} ` +
		`--token={{ .Token }} --pin={{ .Pin }} ` +
		`--gateway-url={{ .Address }}`
}
