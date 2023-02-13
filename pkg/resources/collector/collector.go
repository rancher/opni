package collector

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client    client.Client
	collector *corev1beta1.Collector
	ctx       context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *corev1beta1.Collector,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:    c,
		collector: instance,
		ctx:       ctx,
	}
}
