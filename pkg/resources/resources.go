package resources

import (
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ComponentReconciler reconciler interface
type ComponentReconciler func() (*reconcile.Result, error)

// Resource redeclaration of function with return type kubernetes Object
type Resource func() (runtime.Object, reconciler.DesiredState, error)

// ResourceWithLog redeclaration of function with logging parameter and return type kubernetes Object
type ResourceWithLog func(log logr.Logger) runtime.Object
