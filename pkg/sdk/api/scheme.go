package api

import (
	"github.com/rancher/opni-monitoring/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	util.Must(clientgoscheme.AddToScheme(scheme))
	util.Must(v1beta1.AddToScheme(scheme))
	return scheme
}
