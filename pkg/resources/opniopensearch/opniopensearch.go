package opniopensearch

import (
	"context"
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client         client.Client
	opniOpensearch *v1beta2.OpniOpensearch
	ctx            context.Context
}

func NewReconciler(
	ctx context.Context,
	opniOpensearch *v1beta2.OpniOpensearch,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:         c,
		opniOpensearch: opniOpensearch,
		ctx:            ctx,
	}
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	result := reconciler.CombinedResult{}
	result.Combine(r.ReconcileResource(r.buildOpensearchCluster(), reconciler.StatePresent))
	result.Combine(r.ReconcileResource(r.buildMulticlusterRoleBinding(), reconciler.StatePresent))

	if result.Err != nil || !result.Result.IsZero() {
		return &result.Result, result.Err
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniOpensearch), r.opniOpensearch); err != nil {
			return err
		}

		r.opniOpensearch.Status.Version = &r.opniOpensearch.Spec.Version
		r.opniOpensearch.Status.OpensearchVersion = &r.opniOpensearch.Spec.OpensearchVersion

		return r.client.Status().Update(r.ctx, r.opniOpensearch)
	})
	return &result.Result, err
}

func (r *Reconciler) buildOpensearchCluster() *opsterv1.OpenSearchCluster {
	image := fmt.Sprintf(
		"%s/opensearch:%s-%s",
		r.opniOpensearch.Spec.ImageRepo,
		r.opniOpensearch.Spec.OpensearchVersion,
		r.opniOpensearch.Spec.Version,
	)
	cluster := &opsterv1.OpenSearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.opniOpensearch.Name,
			Namespace: r.opniOpensearch.Namespace,
		},
		Spec: opsterv1.ClusterSpec{
			General: opsterv1.GeneralConfig{
				ImageSpec: &opsterv1.ImageSpec{
					Image: &image,
				},
				Version:          r.opniOpensearch.Spec.Version,
				ServiceName:      fmt.Sprintf("%s-opensearch-svc", r.opniOpensearch.Name),
				HttpPort:         9200,
				SetVMMaxMapCount: true,
			},
			NodePools:  r.opniOpensearch.Spec.NodePools,
			Security:   r.opniOpensearch.Spec.OpensearchSettings.Security,
			Dashboards: r.opniOpensearch.Spec.Dashboards,
		},
	}

	ctrl.SetControllerReference(r.opniOpensearch, cluster, r.client.Scheme())
	return cluster
}

func (r *Reconciler) buildMulticlusterRoleBinding() *v1beta2.MulticlusterRoleBinding {
	binding := &v1beta2.MulticlusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.opniOpensearch.Name,
			Namespace: r.opniOpensearch.Namespace,
		},
		Spec: v1beta2.MulticlusterRoleBindingSpec{
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      r.opniOpensearch.Name,
				Namespace: r.opniOpensearch.Namespace,
			},
			OpensearchExternalURL: r.opniOpensearch.Spec.ExternalURL,
			OpensearchConfig:      r.opniOpensearch.Spec.ClusterConfigSpec,
		},
	}

	ctrl.SetControllerReference(r.opniOpensearch, binding, r.client.Scheme())
	return binding
}
