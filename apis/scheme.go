package apis

import (
	opniaiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opnimonitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	nodev1 "k8s.io/api/node/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
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
	appsv1.AddToScheme,
	batchv1.AddToScheme,
	corev1.AddToScheme,
	networkingv1.AddToScheme,
	nodev1.AddToScheme,
	rbacv1.AddToScheme,
	storagev1.AddToScheme,
	apiextv1.AddToScheme,
	opniv1beta2.AddToScheme,
	opniloggingv1beta1.AddToScheme,
	opnicorev1beta1.AddToScheme,
	opniaiv1beta1.AddToScheme,
	opnimonitoringv1beta1.AddToScheme,
}

func addSchemeBuilders(builders ...func(*runtime.Scheme) error) {
	schemeBuilders = append(schemeBuilders, builders...)
}
