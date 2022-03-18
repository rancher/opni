package logging

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/rancher/opni-monitoring/pkg/core"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
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
	userSuffix, err := generateRandomString(6)
	if err != nil {
		return ErrGenerateCredentialsFailed(err)
	}

	password, err := generateRandomString(20)
	if err != nil {
		return ErrGenerateCredentialsFailed(err)
	}

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

func generateRandomString(n int) (string, error) {
	const letters = "0123456789BCDFGHJKLMNPQRSTVWXZbcdfghjklmnpqrstvwxz"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}
