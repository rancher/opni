package api

import (
	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	util.Must(clientgoscheme.AddToScheme(scheme))
	util.Must(v1beta1.AddToScheme(scheme))
	util.Must(cmv1.AddToScheme(scheme))
	util.Must(monitoringv1.AddToScheme(scheme))
	return scheme
}
