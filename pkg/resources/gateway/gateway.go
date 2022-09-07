package gateway

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx       context.Context
	client    client.Client
	gw        *v1beta2.Gateway
	coreGW    *corev1beta1.Gateway
	logger    *zap.SugaredLogger
	name      string
	namespace string
	spec      corev1beta1.GatewaySpec
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	instance interface{},
) (*Reconciler, error) {
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		logger: logger.New().Named("controller").Named("gateway"),
	}
	switch gw := instance.(type) {
	case *v1beta2.Gateway:
		r.gw = gw
		r.name = gw.Name
		r.namespace = gw.Namespace
		r.spec = convertSpec(gw.Spec)
		return r, nil
	case *corev1beta1.Gateway:
		r.coreGW = gw
		r.name = gw.Name
		r.namespace = gw.Namespace
		r.spec = gw.Spec
		return r, nil
	default:
		return nil, errors.New("invalid gateway instance type")
	}
}

func (r *Reconciler) Reconcile() (retResult reconcile.Result, retErr error) {
	updated, err := r.updateImageStatus()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	if updated {
		return util.Requeue().Result()
	}

	allResources := []resources.Resource{}
	etcdResources, err := r.etcd()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, etcdResources...)
	configMap, err := r.configMap()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, configMap)
	certs, err := r.certs()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, certs...)
	deployment, err := r.deployment()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, deployment)
	services, err := r.services()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, services...)
	rbac, err := r.rbac()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, rbac...)
	allResources = append(allResources, r.serviceMonitor())

	allResources = append(allResources, r.alerting()...)

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.Result()
	}

	// Post initial reconcile we need to build the gateway secret for ingresses
	object, op := r.gatewayIngressSecret()
	if op != nil {
		return op.Result()
	}

	result, err := r.ReconcileResource(object, reconciler.StatePresent)
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	if result != nil {
		return *result, err
	}

	// Post-reconcile, wait for the public service's load balancer to be ready
	if op := r.waitForServiceEndpoints(); op.ShouldRequeue() {
		return op.Result()
	}
	if r.spec.ServiceType == corev1.ServiceTypeLoadBalancer {
		if op := r.waitForLoadBalancer(); op.ShouldRequeue() {
			return op.Result()
		}
	}

	if r.gw != nil {
		r.gw.Status.Ready = true
		if err := r.client.Status().Update(r.ctx, r.gw); err != nil {
			return util.RequeueErr(err).Result()
		}
	}

	if r.coreGW != nil {
		r.coreGW.Status.Ready = true
		if err := r.client.Status().Update(r.ctx, r.coreGW); err != nil {
			return util.RequeueErr(err).Result()
		}
	}

	return util.DoNotRequeue().Result()
}

func (r *Reconciler) statusImage() string {
	if r.gw != nil {
		return r.gw.Status.Image
	}
	if r.coreGW != nil {
		return r.coreGW.Status.Image
	}
	return ""
}

func (r *Reconciler) statusImagePullPolicy() corev1.PullPolicy {
	if r.gw != nil {
		return r.gw.Status.ImagePullPolicy
	}
	if r.coreGW != nil {
		return r.coreGW.Status.ImagePullPolicy
	}
	return corev1.PullIfNotPresent
}

func (r *Reconciler) setOwner(obj client.Object) error {
	if r.gw != nil {
		err := ctrl.SetControllerReference(r.gw, obj, r.client.Scheme())
		if err != nil {
			return err
		}
	}
	if r.coreGW != nil {
		err := ctrl.SetControllerReference(r.coreGW, obj, r.client.Scheme())
		if err != nil {
			return err
		}
	}
	return nil
}

func convertSpec(spec v1beta2.GatewaySpec) corev1beta1.GatewaySpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := corev1beta1.GatewaySpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
