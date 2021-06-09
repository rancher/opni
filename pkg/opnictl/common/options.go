package common

import (
	"time"

	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	TimeoutFlagValue         time.Duration
	NamespaceFlagValue       string
	ContextOverrideFlagValue string
)

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
