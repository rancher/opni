package apis

import (
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// InitScheme adds all the types needed by opni to the provided scheme.
func InitScheme(scheme *runtime.Scheme) {
	for _, f := range schemeBuilders {
		utilruntime.Must(f(scheme))
	}
}

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	InitScheme(scheme)
	return scheme
}

var schemeBuilders = []func(*runtime.Scheme) error{
	apiextv1.AddToScheme,
	opniv1beta2.AddToScheme,
	opnicorev1beta1.AddToScheme,
}

func addSchemeBuilders(builders ...func(*runtime.Scheme) error) {
	schemeBuilders = append(schemeBuilders, builders...)
}
