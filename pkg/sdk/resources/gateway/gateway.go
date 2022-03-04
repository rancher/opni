package gateway

import (
	"context"
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/sdk/resources"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx     context.Context
	client  client.Client
	gateway *v1beta1.Gateway
	logger  *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	gw *v1beta1.Gateway,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client),
		ctx:                ctx,
		client:             client,
		gateway:            gw,
		logger:             logger.New().Named("controller").Named("gateway"),
	}
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	allResources := []resources.Resource{}
	configMap, err := r.configMap()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, configMap)
	certs, err := r.certs()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, certs...)
	deployment, err := r.deployment()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, deployment)
	service, err := r.service()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, service)
	rbac, err := r.rbac()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, rbac...)

	for _, factory := range allResources {
		o, state, err := factory()
		if err != nil {
			return nil, fmt.Errorf("failed to create object: %w", err)
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			return nil, fmt.Errorf("failed to reconcile resource %#T: %w", o, err)
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}
