package indices

import (
	"context"
	"time"

	"emperror.dev/errors"
	"github.com/opensearch-project/opensearch-go"
	"github.com/rancher/opni/apis/v1beta1"
	esapiext "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	headerContentType        = "Content-Type"
	kibanaCrossHeaderType    = "kbn-xsrf"
	securityTenantHeaderType = "securitytenant"

	jsonContentHeader = "application/json"
)

type ExtendedClient struct {
	*opensearch.Client
	ISM *ISMApi
}

type Reconciler struct {
	esReconciler *elasticsearchReconciler
	client       client.Client
	cluster      *v1beta1.OpniCluster
	ctx          context.Context
}

func NewReconciler(ctx context.Context, opniCluster *v1beta1.OpniCluster, c client.Client) *Reconciler {
	// Need to fetch the elasticsearch password from the status
	lg := log.FromContext(ctx)
	password := "admin"
	if err := c.Get(ctx, client.ObjectKeyFromObject(opniCluster), opniCluster); err != nil {
		lg.Error(err, "error fetching cluster status, using default password")
	}
	// TODO this will always be nil the first time an opnicluster is reconciled. Clean up the logic for this.
	if opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef != nil {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, types.NamespacedName{
			Name:      opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef.Name,
			Namespace: opniCluster.Namespace,
		}, secret); err != nil {
			lg.Error(err, "error fetching password secret, using default password")
		}
		password = string(secret.Data[opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef.Key])
	}

	esReconciler := newElasticsearchReconciler(ctx, opniCluster.Namespace, password)
	return &Reconciler{
		cluster:      opniCluster,
		esReconciler: esReconciler,
		ctx:          ctx,
		client:       c,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}
	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := util.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.cluster), r.cluster); err != nil {
				return err
			}
			r.cluster.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.cluster.Status.IndexState = v1beta1.OpniClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.cluster.Status.IndexState = v1beta1.OpniClusterStateWorking
				}
			} else if len(r.cluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.cluster.Status.IndexState = v1beta1.OpniClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.cluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	kibanaDeployment := &appsv1.Deployment{}
	lg.V(1).Info("reconciling elastic indices")
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      "opni-es-kibana",
		Namespace: r.cluster.Namespace,
	}, kibanaDeployment)
	if err != nil {
		conditions = append(conditions, err.Error())
		retErr = errors.Combine(retErr, err)
		return
	}

	if kibanaDeployment.Status.AvailableReplicas < 1 {
		lg.Info("waiting for elastic stack")
		conditions = append(conditions, "waiting for elastic cluster to be available")
		retResult = &reconcile.Result{RequeueAfter: 5 * time.Second}
		return
	}

	for _, policy := range []esapiext.ISMPolicySpec{
		opniLogPolicy,
		opniDrainModelStatusPolicy,
	} {
		err = r.esReconciler.reconcileISM(&policy)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	for _, template := range []esapiext.IndexTemplateSpec{
		opniLogTemplate,
		drainStatusTemplate,
	} {
		err = r.esReconciler.maybeCreateIndexTemplate(&template)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	for prefix, alias := range map[string]string{
		logIndexPrefix:         logIndexAlias,
		drainStatusIndexPrefix: drainStatusIndexAlias,
	} {
		err = r.esReconciler.maybeBootstrapIndex(prefix, alias)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	err = r.esReconciler.maybeCreateIndex(normalIntervalIndexName, normalIntervalIndexSettings)
	if err != nil {
		conditions = append(conditions, err.Error())
		retErr = errors.Combine(retErr, err)
	}

	err = r.esReconciler.importKibanaObjects()
	if err != nil {
		conditions = append(conditions, err.Error())
		retErr = errors.Combine(retErr, err)
	}

	return
}
