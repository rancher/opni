package downstreamcluster

import (
	"errors"
	"fmt"
	"time"

	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources/multicluster"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	ErrorMissingUserSecret = errors.New("user secret not set")

	indexUser = osapiext.UserSpec{}
)

func (r *Reconciler) ReconcileOpensearchUsers(opensearchCluster *opensearchv1beta1.OpensearchCluster) (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)

	// We should have a user secret when we get to here.  If not requeue the reconciliation loop
	if r.downstreamCluster.Status.IndexUserSecretRef == nil {
		lg.V(1).Error(ErrorMissingUserSecret, "indexUserSecret not set, reqeueing")
		retResult = &reconcile.Result{
			RequeueAfter: time.Second * 5,
		}
		return
	}

	indexUser.Attributes = map[string]string{
		"cluster": r.downstreamCluster.Labels[v2beta1.IDLabel],
	}

	secret := &corev1.Secret{}

	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.downstreamCluster.Status.IndexUserSecretRef.Name,
		Namespace: r.downstreamCluster.Namespace,
	}, secret)
	if retErr != nil {
		return
	}

	indexUser.UserName = secret.Name
	indexUser.Password = string(secret.Data["password"])

	osPassword, retErr := multicluster.FetchOpensearchAdminPassword(r.ctx, r.client, opensearchCluster)
	if retErr != nil {
		return
	}

	reconciler := opensearch.NewReconciler(r.ctx, opensearchCluster.Namespace, osPassword, fmt.Sprintf("%s-os", opensearchCluster.Name))

	retErr = reconciler.MaybeCreateUser(indexUser)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeUpdateRolesMapping("cluster_index", indexUser.UserName)

	return
}
