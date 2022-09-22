package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/spf13/pflag"
	"github.com/ttacon/chalk"
)

func main() {
	gin.SetMode(gin.TestMode)
	var enableGateway, enableEtcd, enableCortex, enableCortexClusterDriver bool
	var remoteGatewayAddress, remoteKubeconfig string
	var agentIdSeed int64

	pflag.BoolVar(&enableGateway, "enable-gateway", true, "enable gateway")
	pflag.BoolVar(&enableEtcd, "enable-etcd", true, "enable etcd")
	pflag.BoolVar(&enableCortex, "enable-cortex", true, "enable cortex")
	pflag.StringVar(&remoteGatewayAddress, "remote-gateway-address", "", "remote gateway address")
	pflag.StringVar(&remoteKubeconfig, "remote-kubeconfig", "", "remote kubeconfig (for accessing the management api)")
	pflag.Int64Var(&agentIdSeed, "agent-id-seed", 0, "random seed used for generating agent ids. if unset, uses a random seed.")
	pflag.BoolVar(&enableCortexClusterDriver, "enable-cortex-cluster-driver", false, "enable cortex cluster driver")

	pflag.Parse()

	if !pflag.Lookup("agent-id-seed").Changed {
		agentIdSeed = time.Now().UnixNano()
	}

	defaultAgentOpts := []test.StartAgentOption{}
	if remoteGatewayAddress != "" {
		fmt.Fprintln(os.Stdout, chalk.Blue.Color("disabling gateway and cortex since remote gateway address is set"))
		enableGateway = false
		enableCortex = false
		defaultAgentOpts = append(defaultAgentOpts, test.WithRemoteGatewayAddress(remoteGatewayAddress))
	}
	if remoteKubeconfig != "" {
		defaultAgentOpts = append(defaultAgentOpts, test.WithRemoteKubeconfig(remoteKubeconfig))
	}

	tracing.Configure("testenv")

	test.StartStandaloneTestEnvironment(
		test.WithEnableGateway(enableGateway),
		test.WithEnableEtcd(enableEtcd),
		test.WithEnableCortex(enableCortex),
		test.WithDefaultAgentOpts(defaultAgentOpts...),
		test.WithAgentIdSeed(agentIdSeed),
		test.WithEnableCortexClusterDriver(enableCortexClusterDriver),
	)
}
