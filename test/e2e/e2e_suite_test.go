package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testutil"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Tests")
}

var (
	testEnv    *test.Environment
	stopEnv    context.CancelFunc
	k8sClient  client.Client
	restConfig *rest.Config
	mgmtClient managementv1.ManagementClient
)

type terraformOutput struct {
	GatewayURL struct {
		Value string `json:"value"`
	} `json:"gateway_url"`
	Kubeconfig struct {
		Value string `json:"value"`
	} `json:"kubeconfig"`
}

var _ = BeforeSuite(func() {
	testEnv = &test.Environment{
		TestBin: "../../testbin/bin",
	}
	// export TF_OUTPUT=$(terraform output -json)
	tfOutputEnv, ok := os.LookupEnv("TF_OUTPUT")
	if !ok {
		Fail("TF_OUTPUT env variable is not set")
	}
	var tfOutput terraformOutput
	Expect(json.Unmarshal([]byte(tfOutputEnv), &tfOutput)).To(Succeed())

	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(tfOutput.Kubeconfig.Value))
	Expect(err).NotTo(HaveOccurred())

	ctx, ca := context.WithCancel(context.Background())
	DeferCleanup(ca)

	internalPorts, err := testutil.PortForward(ctx, types.NamespacedName{
		Namespace: "opni",
		Name:      "opni-monitoring-internal",
	}, []string{
		"11090",
	}, restConfig, apis.NewScheme())
	Expect(err).NotTo(HaveOccurred())
	Expect(len(internalPorts)).To(Equal(1))

	Expect(testEnv.Start(
		test.WithEnableCortex(false),
		test.WithEnableGateway(false),
		test.WithDefaultAgentOpts(test.WithRemoteGatewayAddress(tfOutput.GatewayURL.Value)),
	)).To(Succeed())

	mgmtClient, err = clients.NewManagementClient(ctx,
		clients.WithAddress(fmt.Sprintf("127.0.0.1:%d", internalPorts[0].Local)),
	)
	Expect(err).NotTo(HaveOccurred())
})
