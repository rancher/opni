package opniopensearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client                client.Client
	opniOpensearch        *v1beta2.OpniOpensearch
	loggingOpniOpensearch *loggingv1beta1.OpniOpensearch
	instanceName          string
	instanceNamespace     string
	spec                  loggingv1beta1.OpniOpensearchSpec
	ctx                   context.Context
}

func NewReconciler(
	ctx context.Context,
	instance interface{},
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) (*Reconciler, error) {
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client: c,
		ctx:    ctx,
	}
	switch cluster := instance.(type) {
	case *v1beta2.OpniOpensearch:
		r.opniOpensearch = cluster
		r.instanceName = cluster.Name
		r.instanceNamespace = cluster.Namespace
		r.spec = convertSpec(cluster.Spec)
	case *loggingv1beta1.OpniOpensearch:
		r.loggingOpniOpensearch = cluster
		r.instanceName = cluster.Name
		r.instanceNamespace = cluster.Namespace
		r.spec = cluster.Spec
	default:
		return nil, errors.New("invalid opniopensearch")
	}
	return r, nil
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	result := reconciler.CombinedResult{}
	result.Combine(r.ReconcileResource(r.buildOpensearchCluster(), reconciler.StatePresent))
	result.Combine(r.ReconcileResource(r.buildMulticlusterRoleBinding(), reconciler.StatePresent))

	if result.Err != nil || !result.Result.IsZero() {
		return &result.Result, result.Err
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if r.opniOpensearch != nil {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniOpensearch), r.opniOpensearch); err != nil {
				return err
			}

			r.opniOpensearch.Status.Version = &r.opniOpensearch.Spec.Version
			r.opniOpensearch.Status.OpensearchVersion = &r.opniOpensearch.Spec.OpensearchVersion

			return r.client.Status().Update(r.ctx, r.opniOpensearch)
		}
		if r.loggingOpniOpensearch != nil {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingOpniOpensearch), r.loggingOpniOpensearch); err != nil {
				return err
			}

			r.loggingOpniOpensearch.Status.Version = &r.loggingOpniOpensearch.Spec.Version
			r.loggingOpniOpensearch.Status.OpensearchVersion = &r.loggingOpniOpensearch.Spec.OpensearchVersion

			return r.client.Status().Update(r.ctx, r.loggingOpniOpensearch)
		}
		return errors.New("no opniopensearch to update")
	})
	return &result.Result, err
}

func (r *Reconciler) buildOpensearchCluster() *opsterv1.OpenSearchCluster {
	// Set default image version
	version := r.spec.Version
	if version == "unversioned" {
		version = "0.5.4"
	}

	image := fmt.Sprintf(
		"%s/opensearch:%s-%s",
		r.spec.ImageRepo,
		r.spec.OpensearchVersion,
		version,
	)
	cluster := &opsterv1.OpenSearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.instanceName,
			Namespace: r.instanceNamespace,
		},
		Spec: opsterv1.ClusterSpec{
			General: opsterv1.GeneralConfig{
				ImageSpec: &opsterv1.ImageSpec{
					Image: &image,
				},
				Version:          r.spec.OpensearchVersion,
				ServiceName:      fmt.Sprintf("%s-opensearch-svc", r.instanceName),
				HttpPort:         9200,
				SetVMMaxMapCount: true,
			},
			NodePools:  r.spec.NodePools,
			Security:   r.spec.OpensearchSettings.Security,
			Dashboards: r.spec.Dashboards,
		},
	}

	r.setOwner(cluster)
	return cluster
}

func (r *Reconciler) buildMulticlusterRoleBinding() runtime.Object {
	if r.opniOpensearch != nil {
		binding := &v1beta2.MulticlusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.instanceName,
				Namespace: r.instanceNamespace,
			},
			Spec: v1beta2.MulticlusterRoleBindingSpec{
				OpensearchCluster: &opnimeta.OpensearchClusterRef{
					Name:      r.instanceName,
					Namespace: r.instanceNamespace,
				},
				OpensearchExternalURL: r.spec.ExternalURL,
				OpensearchConfig:      (*v1beta2.ClusterConfigSpec)(r.spec.ClusterConfigSpec),
			},
		}

		r.setOwner(binding)
		return binding
	}
	if r.loggingOpniOpensearch != nil {
		binding := &loggingv1beta1.MulticlusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.instanceName,
				Namespace: r.instanceNamespace,
			},
			Spec: loggingv1beta1.MulticlusterRoleBindingSpec{
				OpensearchCluster: &opnimeta.OpensearchClusterRef{
					Name:      r.instanceName,
					Namespace: r.instanceNamespace,
				},
				OpensearchExternalURL: r.spec.ExternalURL,
				OpensearchConfig:      r.spec.ClusterConfigSpec,
			},
		}

		r.setOwner(binding)
		return binding
	}
	return nil
}

func (r *Reconciler) setOwner(obj client.Object) error {
	if r.opniOpensearch != nil {
		return ctrl.SetControllerReference(r.opniOpensearch, obj, r.client.Scheme())
	}
	if r.loggingOpniOpensearch != nil {
		return ctrl.SetControllerReference(r.loggingOpniOpensearch, obj, r.client.Scheme())
	}
	return errors.New("no opniopensearch object")
}

func convertSpec(spec v1beta2.OpniOpensearchSpec) loggingv1beta1.OpniOpensearchSpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := loggingv1beta1.OpniOpensearchSpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
