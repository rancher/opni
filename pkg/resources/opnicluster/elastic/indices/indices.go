package indices

import (
	"context"

	"emperror.dev/errors"
	opensearchv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	esapiext "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ISMChangeVersion = "1.1.0"
)

type Reconciler struct {
	ReconcilerOptions
	osReconciler *opensearch.Reconciler
	client       client.Client
	cluster      *aiv1beta1.OpniCluster
	opensearch   *opensearchv1.OpenSearchCluster
	ctx          context.Context
}

type ReconcilerOptions struct {
	certMgr certs.OpensearchCertReader
}

type ReconcilerOption func(*ReconcilerOptions)

func (o *ReconcilerOptions) apply(opts ...ReconcilerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithCertManager(certMgr certs.OpensearchCertReader) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.certMgr = certMgr
	}
}

func NewReconciler(
	ctx context.Context,
	instance *aiv1beta1.OpniCluster,
	opensearchCluster *opensearchv1.OpenSearchCluster,
	c client.Client,
	opts ...ReconcilerOption,
) (*Reconciler, error) {
	// Need to fetch the elasticsearch password from the status
	reconciler := &Reconciler{
		ctx:        ctx,
		client:     c,
		cluster:    instance,
		opensearch: opensearchCluster,
	}
	options := ReconcilerOptions{}
	options.apply(opts...)
	if options.certMgr == nil {
		options.certMgr = certs.NewCertMgrOpensearchCertManager(
			ctx,
			certs.WithNamespace(opensearchCluster.Namespace),
			certs.WithCluster(opensearchCluster.Name),
		)
	}

	var err error
	reconciler.osReconciler, err = opensearch.NewReconciler(
		ctx,
		opensearch.ReconcilerConfig{
			CertReader:            options.certMgr,
			OpensearchServiceName: opensearchCluster.Spec.General.ServiceName,
		},
	)
	if err != nil {
		return nil, err
	}

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

	policies := []esapiext.ISMPolicySpec{
		opniDrainModelStatusPolicy,
		opniMetricPolicy,
	}
	for _, policy := range policies {
		err := r.osReconciler.ReconcileISM(policy)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	templates := []esapiext.IndexTemplateSpec{
		drainStatusTemplate,
		opniMetricTemplate,
	}

	for _, template := range templates {
		err := r.osReconciler.MaybeCreateIndexTemplate(template)
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	prefixes := map[string]string{
		drainStatusIndexPrefix: drainStatusIndexAlias,
		metricIndexPrefix:      metricIndexAlias,
	}

	for prefix, alias := range prefixes {
		err := r.osReconciler.MaybeBootstrapIndex(prefix, alias, []string{})
		if err != nil {
			conditions = append(conditions, err.Error())
			retErr = errors.Combine(retErr, err)
			return
		}
	}

	err := r.osReconciler.MaybeCreateIndex(logTemplateIndexName, logTemplateIndexSettings)
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
