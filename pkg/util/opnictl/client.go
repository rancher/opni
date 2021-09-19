// Package opnictl contains various utility and helper functions that are used
// by the Opnictl CLI.
package opnictl

import (
	"fmt"
	"os"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni/apis/demo/v1alpha1"
	"github.com/rancher/opni/apis/v1beta1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientOptions can be passed to some of the functions in this package when
// creating clients and/or client configurations.
type ClientOptions struct {
	overrides    *clientcmd.ConfigOverrides
	explicitPath string
}

type ClientOption func(*ClientOptions)

func (o *ClientOptions) Apply(opts ...ClientOption) {
	for _, op := range opts {
		op(o)
	}
}

// WithConfigOverrides allows overriding specific kubeconfig fields from the
// user's loaded kubeconfig.
func WithConfigOverrides(overrides *clientcmd.ConfigOverrides) ClientOption {
	return func(o *ClientOptions) {
		o.overrides = overrides
	}
}

func WithExplicitPath(path string) ClientOption {
	return func(o *ClientOptions) {
		o.explicitPath = path
	}
}

// CreateClientOrDie constructs a new controller-runtime client, or exit
// with a fatal error if an error occurs.
func CreateClientOrDie(opts ...ClientOption) (*rest.Config, client.Client) {
	scheme := CreateScheme()
	clientConfig := LoadClientConfig(opts...)

	cli, err := client.New(clientConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	return clientConfig, cli
}

// LoadClientConfig loads the user's kubeconfig using the same logic as kubectl.
func LoadClientConfig(opts ...ClientOption) *rest.Config {
	options := ClientOptions{}
	options.Apply(opts...)

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	// the loading rules check for empty string in the ExplicitPath, so it is
	// safe to always set this, it defaults to empty string.
	rules.ExplicitPath = options.explicitPath
	kubeconfig, err := rules.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	clientConfig, err := clientcmd.NewDefaultClientConfig(
		*kubeconfig, options.overrides).ClientConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	return clientConfig
}

// CreateScheme creates a new scheme with the types necessary for opnictl.
func CreateScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(loggingv1beta1.AddToScheme(scheme))
	return scheme
}
