package opnictl

import (
	"fmt"
	"os"

	"github.com/rancher/opni/api/v1alpha1"
	"github.com/rancher/opni/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateClientOrDie() client.Client {
	scheme := CreateScheme()
	clientConfig := LoadClientConfig()

	cli, err := client.New(clientConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	return cli
}

func LoadClientConfig() *rest.Config {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig, err := rules.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	clientConfig, err := clientcmd.NewDefaultClientConfig(*kubeconfig, nil).
		ClientConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	return clientConfig
}

func CreateScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	return scheme
}
