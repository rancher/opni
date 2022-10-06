package indices

import (
	"context"
	"fmt"
	"time"

	"emperror.dev/errors"
	"github.com/hashicorp/go-version"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources/opnicluster"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/opensearch"
	esapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
	cluster      *v1beta2.OpniCluster
	aiCluster    *aiv1beta1.OpniCluster
	opensearch   *opensearchv1.OpenSearchCluster
	namespace    string
	spec         v1beta2.OpniClusterSpec
	ctx          context.Context
}

func NewReconciler(ctx context.Context, instance interface{}, c client.Client) (*Reconciler, error) {
	// Need to fetch the elasticsearch password from the status
	reconciler := &Reconciler{
		ctx:    ctx,
		client: c,
	}
	lg := log.FromContext(ctx)
	password := "admin"
	switch opniCluster := instance.(type) {
	case *v1beta2.OpniCluster:
		reconciler.cluster = opniCluster
		if err := c.Get(ctx, client.ObjectKeyFromObject(reconciler.cluster), reconciler.cluster); err != nil {
			lg.Error(err, "error fetching cluster status, using default password")
		}
		// TODO this will always be nil the first time an opnicluster is reconciled. Clean up the logic for this.
		if reconciler.cluster.Status.Auth.OpensearchAuthSecretKeyRef != nil {
			secret := &corev1.Secret{}
			if err := c.Get(ctx, types.NamespacedName{
				Name:      reconciler.cluster.Status.Auth.OpensearchAuthSecretKeyRef.Name,
				Namespace: reconciler.cluster.Namespace,
			}, secret); err != nil {
				lg.Error(err, "error fetching password secret, using default password")
			}
			password = string(secret.Data[reconciler.cluster.Status.Auth.OpensearchAuthSecretKeyRef.Key])
		}
		reconciler.spec = opniCluster.Spec
		reconciler.namespace = opniCluster.Namespace
	case *aiv1beta1.OpniCluster:
		reconciler.aiCluster = opniCluster
		if err := c.Get(ctx, client.ObjectKeyFromObject(reconciler.aiCluster), reconciler.aiCluster); err != nil {
			lg.Error(err, "error fetching cluster status, using default password")
		}
		if reconciler.aiCluster.Status.Auth.OpensearchAuthSecretKeyRef != nil {
			secret := &corev1.Secret{}
			if err := c.Get(ctx, types.NamespacedName{
				Name:      reconciler.aiCluster.Status.Auth.OpensearchAuthSecretKeyRef.Name,
				Namespace: reconciler.aiCluster.Namespace,
			}, secret); err != nil {
				lg.Error(err, "error fetching password secret, using default password")
			}
			password = string(secret.Data[reconciler.aiCluster.Status.Auth.OpensearchAuthSecretKeyRef.Key])
		}
		reconciler.spec = opnicluster.ConvertSpec(opniCluster.Spec)
		reconciler.namespace = opniCluster.Namespace
	default:
		return nil, errors.New("invalid opnicluster type")
	}

	// Handle external opensearch cluster
	username := "admin"
	osSvcName := "opni-es-client"
	kbSvcName := "opni-es-kibana"

	if reconciler.spec.Opensearch.ExternalOpensearch != nil {
		opensearchCluster := &opensearchv1.OpenSearchCluster{}
		err := c.Get(ctx, reconciler.spec.Opensearch.ExternalOpensearch.ObjectKeyFromRef(), opensearchCluster)
		if err != nil {
			lg.Error(err, "failed to fetch opensearch, index reconciliation will continue with defaults")
		}

		reconciler.opensearch = opensearchCluster

		username, _, err = helpers.UsernameAndPassword(ctx, c, opensearchCluster)
		if err != nil {
			lg.Error(err, "fetching username from opensearch failed")
		}

		osSvcName = opensearchCluster.Spec.General.ServiceName
		kbSvcName = fmt.Sprintf("%s-dashboards", opensearchCluster.Spec.General.ServiceName)
	}

	reconciler.osReconciler = opensearch.NewReconciler(ctx, reconciler.namespace, username, password, osSvcName, kbSvcName)
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
			if r.cluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.cluster), r.cluster); err != nil {
					return err
				}
				r.cluster.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.cluster.Status.OpensearchState.IndexState = v1beta2.OpniClusterStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.cluster.Status.OpensearchState.IndexState = v1beta2.OpniClusterStateWorking
					}
				} else if len(r.cluster.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.cluster.Status.OpensearchState.IndexState = v1beta2.OpniClusterStateReady
				}
				return r.client.Status().Update(r.ctx, r.cluster)
			}
			if r.aiCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.aiCluster), r.aiCluster); err != nil {
					return err
				}
				r.aiCluster.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.aiCluster.Status.IndexState = aiv1beta1.OpniClusterStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.aiCluster.Status.IndexState = aiv1beta1.OpniClusterStateWorking
					}
				} else if len(r.aiCluster.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.aiCluster.Status.IndexState = aiv1beta1.OpniClusterStateReady
				}
				return r.client.Status().Update(r.ctx, r.aiCluster)
			}
			return errors.New("no opnicluster to update")
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	oldVersion := false
	changeVersion, _ := version.NewVersion(ISMChangeVersion)

	var desiredVersion *version.Version
	var err error
	if r.opensearch != nil {
		desiredVersion, err = version.NewVersion(r.opensearch.Spec.General.Version)
	} else {
		desiredVersion, err = version.NewVersion(r.spec.Opensearch.Version)
	}

	if err != nil {
		lg.V(1).Error(err, "failed to parse opensearch version")
	} else {
		oldVersion = desiredVersion.LessThan(changeVersion)
	}

	if r.opensearch == nil {
		kibanaDeployment := &appsv1.Deployment{}
		lg.V(1).Info("reconciling elastic indices")
		err = r.client.Get(r.ctx, types.NamespacedName{
			Name:      "opni-es-kibana",
			Namespace: r.namespace,
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
	}

	var policies []interface{}
	if oldVersion {
		if lo.FromPtrOr(r.spec.Opensearch.EnableLogIndexManagement, true) {
			policies = append(policies, oldOpniLogPolicy)
		}
		policies = append(policies, oldOpniDrainModelStatusPolicy)
		policies = append(policies, oldOpniMetricPolicy)
	} else {
		if lo.FromPtrOr(r.spec.Opensearch.EnableLogIndexManagement, true) {
			policies = append(policies, OpniLogPolicy)
		}
		policies = append(policies, opniDrainModelStatusPolicy)
		policies = append(policies, opniMetricPolicy)
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
	}

	if lo.FromPtrOr(r.spec.Opensearch.EnableLogIndexManagement, true) {
		templates = append(templates, OpniLogTemplate)
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
	}

	if lo.FromPtrOr(r.spec.Opensearch.EnableLogIndexManagement, true) {
		prefixes[LogIndexPrefix] = LogIndexAlias
	}

	for prefix, alias := range prefixes {
		err = r.osReconciler.MaybeBootstrapIndex(prefix, alias, OldIndexPrefixes)
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
