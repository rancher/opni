package resources

import (
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/go-logr/logr"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ComponentReconciler reconciler interface
type ComponentReconciler func() (*reconcile.Result, error)

// Resource redeclaration of function with return type kubernetes Object
type Resource func() (runtime.Object, reconciler.DesiredState, error)

// ResourceWithLog redeclaration of function with logging parameter and return type kubernetes Object
type ResourceWithLog func(log logr.Logger) runtime.Object

func Absent(obj client.Object) Resource {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		return obj, reconciler.StateAbsent, nil
	}
}

func Present(obj client.Object) Resource {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		return obj, reconciler.StatePresent, nil
	}
}

func Created(obj client.Object) Resource {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		return obj, reconciler.StateCreated, nil
	}
}

func Error(obj client.Object, err error) Resource {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		return obj, reconciler.StatePresent, err
	}
}

func PresentIff(condition bool, obj client.Object) Resource {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		if condition {
			return obj, reconciler.StatePresent, nil
		}
		return obj, reconciler.StateAbsent, nil
	}
}

func CreatedIff(condition bool, obj client.Object) Resource {
	return func() (runtime.Object, reconciler.DesiredState, error) {
		if condition {
			return obj, reconciler.StateCreated, nil
		}
		return obj, reconciler.StateAbsent, nil
	}
}

func ReconcileAll(r reconciler.ResourceReconciler, resources []Resource) k8sutil.RequeueOp {
	for _, factory := range resources {
		o, state, err := factory()
		if err != nil {
			return k8sutil.RequeueErr(fmt.Errorf("failed to create object: %w", err))
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			return k8sutil.RequeueErr(fmt.Errorf("failed to reconcile resource %#T: %w", o, err))
		}
		if result != nil {
			return k8sutil.LoadResult(result, err)
		}
	}
	return k8sutil.DoNotRequeue()
}
