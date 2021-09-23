package apis

import (
	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/apis/demo/v1alpha1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opninfdv1 "github.com/rancher/opni/apis/nfd/v1"
	opninvidiav1 "github.com/rancher/opni/apis/nvidia/v1"
	"github.com/rancher/opni/apis/v1beta1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// InitScheme adds all the types needed by opni to the provided scheme.
func InitScheme(scheme *runtime.Scheme) {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(helmv1.AddToScheme(scheme))
	utilruntime.Must(opniloggingv1beta1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(opninvidiav1.AddToScheme(scheme))
	utilruntime.Must(opninfdv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}
