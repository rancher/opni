package pretrainedmodel

import (
	"context"
	"encoding/json"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/hyperparameters"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx               context.Context
	client            client.Client
	model             *v1beta2.PretrainedModel
	aiModel           *aiv1beta1.PretrainedModel
	instanceName      string
	instanceNamespace string
	spec              aiv1beta1.PretrainedModelSpec
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	instance interface{},
	opts ...reconciler.ResourceReconcilerOption,
) (*Reconciler, error) {
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		ctx:    ctx,
		client: client,
	}

	switch model := instance.(type) {
	case *v1beta2.PretrainedModel:
		r.instanceName = model.Name
		r.instanceNamespace = model.Namespace
		r.spec = convertSpec(model.Spec)
		r.model = model
	case *aiv1beta1.PretrainedModel:
		r.instanceName = model.Name
		r.instanceNamespace = model.Namespace
		r.spec = model.Spec
		r.aiModel = model
	default:
		return nil, errors.New("invalid pretrained model instance")
	}

	return r, nil
}

func (r *Reconciler) hyperparameters() (runtime.Object, reconciler.DesiredState, error) {
	cm, err := hyperparameters.GenerateHyperparametersConfigMap(r.instanceName, r.instanceNamespace, r.spec.Hyperparameters)
	if err != nil {
		return nil, nil, err
	}
	cm.Labels[resources.PretrainedModelLabel] = r.instanceName
	err = r.setOwner(&cm)
	return &cm, reconciler.StatePresent, err
}

func (r *Reconciler) setOwner(obj client.Object) error {
	if r.model != nil {
		return ctrl.SetControllerReference(r.model, obj, r.client.Scheme())
	}
	if r.aiModel != nil {
		return ctrl.SetControllerReference(r.aiModel, obj, r.client.Scheme())
	}
	return errors.New("no pretrained model instance")
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	for _, factory := range []resources.Resource{
		r.hyperparameters,
	} {
		o, state, err := factory()
		if err != nil {
			return nil, errors.WrapIf(err, "failed to create object")
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			return nil, errors.WrapWithDetails(err, "failed to reconcile resource",
				"resource", o.GetObjectKind().GroupVersionKind())
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}

func convertSpec(spec v1beta2.PretrainedModelSpec) aiv1beta1.PretrainedModelSpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := aiv1beta1.PretrainedModelSpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
