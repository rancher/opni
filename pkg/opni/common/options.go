package common

import (
	"fmt"
	"os"
	"time"

	cliutil "github.com/rancher/opni/pkg/opni/util"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// These constants are available to all opnictl sub-commands and are filled
// in by the root command using persistent flags.

var (
	TimeoutFlagValue         time.Duration
	NamespaceFlagValue       string
	ContextOverrideFlagValue string
	ExplicitPathFlagValue    string
	K8sClient                client.Client
	RestConfig               *rest.Config
	APIConfig                *api.Config
)

const (
	DefaultOpniNamespace                  = "opni-system"
	DefaultOpniDemoName                   = "opni-demo"
	DefaultOpniDemoNamespace              = "opni-demo"
	DefaultOpniDemoMinioAccessKey         = "minioadmin"
	DefaultOpniDemoMinioSecretKey         = "minioadmin"
	DefaultOpniDemoMinioVersion           = "8.0.10"
	DefaultOpniDemoNatsVersion            = "2.2.1"
	DefaultOpniDemoNatsPassword           = "password"
	DefaultOpniDemoNatsReplicas           = 3
	DefaultOpniDemoNatsMaxPayload         = 10485760
	DefaultOpniDemoNvidiaVersion          = "1.0.0-beta6"
	DefaultOpniDemoElasticUser            = "admin"
	DefaultOpniDemoElasticPassword        = "admin"
	DefaultOpniDemoNulogServiceCPURequest = "1"
	DefaultOpniDemoQuickstart             = false
)

// TODO: Cleanup - this is never called and won't work anyway as the flags are never bound
// MaybeContextOverride will return 0 or 1 ClientOptions, depending on if the
// user provided a specific kubectl context or not.
func MaybeContextOverride() []cliutil.ClientOption {
	if ContextOverrideFlagValue != "" {
		return []cliutil.ClientOption{
			cliutil.WithConfigOverrides(&clientcmd.ConfigOverrides{
				CurrentContext: NamespaceFlagValue,
			}),
		}
	}
	return []cliutil.ClientOption{}
}

func LoadDefaultClientConfig() {
	APIConfig, RestConfig, K8sClient = cliutil.CreateClientOrDie(
		append(MaybeContextOverride(), cliutil.WithExplicitPath(ExplicitPathFlagValue))...)
}

func GetClientOrDie(inCluster bool) client.Client {
	if inCluster {
		scheme := cliutil.CreateScheme()
		config, err := rest.InClusterConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		k8sClient, err := client.New(config, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return k8sClient
	}
	_, _, k8sClient := cliutil.CreateClientOrDie(
		append(MaybeContextOverride(), cliutil.WithExplicitPath(ExplicitPathFlagValue))...)
	return k8sClient
}
