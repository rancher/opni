package indices

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/hashicorp/go-version"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/opensearch"
	esapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ISMChangeVersion = "1.1.0"
)

type Reconciler struct {
	osReconciler *opensearch.Reconciler
	client       client.Client
	cluster      *aiv1beta1.OpniCluster
	opensearch   *opensearchv1.OpenSearchCluster
	ctx          context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *aiv1beta1.OpniCluster,
	opensearchCluster *opensearchv1.OpenSearchCluster,
	c client.Client,
) (*Reconciler, error) {
	// Need to fetch the elasticsearch password from the status
	reconciler := &Reconciler{
		ctx:        ctx,
		client:     c,
		cluster:    instance,
		opensearch: opensearchCluster,
	}
	lg := log.FromContext(ctx)

	username, password, err := helpers.UsernameAndPassword(ctx, c, opensearchCluster)
	if err != nil {
		lg.Error(err, "fetching username from opensearch failed")
	}

	osSvcName := opensearchCluster.Spec.General.ServiceName
	kbSvcName := fmt.Sprintf("%s-dashboards", opensearchCluster.Spec.General.ServiceName)

	reconciler.osReconciler = opensearch.NewReconciler(
		ctx,
		reconciler.cluster.Spec.Opensearch.Namespace,
		username,
		password,
		osSvcName,
		kbSvcName,
	)
	return reconciler, nil
}

// TODO the bulk of this should be moved to multicluster rolebindings
func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}
	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.cluster), r.cluster); err != nil {
				return err
			}
			r.cluster.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.cluster.Status.IndexState = aiv1beta1.OpniClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.cluster.Status.IndexState = aiv1beta1.OpniClusterStateWorking
				}
			} else if len(r.cluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.cluster.Status.IndexState = aiv1beta1.OpniClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.cluster)
		})
		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	oldVersion := false
	changeVersion, _ := version.NewVersion(ISMChangeVersion)

	desiredVersion, err := version.NewVersion(r.opensearch.Spec.General.Version)
	if err != nil {
		lg.V(1).Error(err, "failed to parse opensearch version")
	} else {
		oldVersion = desiredVersion.LessThan(changeVersion)
	}

	var policies []interface{}
	if oldVersion {
		policies = append(policies, oldOpniDrainModelStatusPolicy)
		policies = append(policies, oldOpniMetricPolicy)
	} else {
		policies = append(policies, opniDrainModelStatusPolicy)
		policies = append(policies, opniMetricPolicy)
		policies = append(policies, opniLogTemplatePolicy)
	}
	for _, policy := range policies {
		err = r.osReconciler.ReconcileISM(policy)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	templates := []esapiext.IndexTemplateSpec{
		drainStatusTemplate,
		opniMetricTemplate,
		logTemplate,
	}

	for _, template := range templates {
		err = r.osReconciler.MaybeCreateIndexTemplate(template)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	prefixes := map[string]string{
		drainStatusIndexPrefix: drainStatusIndexAlias,
		metricIndexPrefix:      metricIndexAlias,
		logTemplateIndexPrefix: logTemplateIndexAlias,
	}

	for prefix, alias := range prefixes {
		err = r.osReconciler.MaybeBootstrapIndex(prefix, alias, []string{})
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	err = r.osReconciler.MaybeCreateIndex(logTemplateIndexName, logTemplateIndexSettings)
	if err != nil {
		conditions = append(conditions, err.Error())
		retErr = errors.Combine(retErr, err)
	}

	err = r.osReconciler.MaybeCreateIndex(normalIntervalIndexName, normalIntervalIndexSettings)
	if err != nil {
		conditions = append(conditions, err.Error())
		retErr = errors.Combine(retErr, err)
	}

	return
}
