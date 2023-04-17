//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/rancher/opni/apis"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"google.golang.org/protobuf/types/known/emptypb"
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
	testEnv            *test.Environment
	stopEnv            context.CancelFunc
	k8sClient          client.Client
	restConfig         *rest.Config
	mgmtClient         managementv1.ManagementClient
	adminClient        cortexadmin.CortexAdminClient
	cortexOpsClient    cortexops.CortexOpsClient
	loggingAdminClient loggingadmin.LoggingAdminClient
	outputs            StackOutputs
	gatewayAddress     string
)

type StackOutputs struct {
	GatewayURL        string `json:"gateway_url"`
	GrafanaURL        string `json:"grafana_url"`
	OpensearchURL     string `json:"opensearch_url"`
	Kubeconfig        string `json:"kubeconfig"`
	OAuthClientID     string `json:"oauth_client_id"`
	OAuthClientSecret string `json:"oauth_client_secret"`
	OAuthIssuerURL    string `json:"oauth_issuer_url"`
	S3Bucket          string `json:"s3_bucket"`
	S3Endpoint        string `json:"s3_endpoint"`
	S3Region          string `json:"s3_region"`
	S3AccessKeyId     string `json:"s3_access_key_id"`
	S3SecretAccessKey string `json:"s3_secret_access_key"`
}

var _ = BeforeSuite(func() {
	testEnv = &test.Environment{}

	if value, ok := os.LookupEnv("STACK_OUTPUTS"); ok {
		Expect(json.Unmarshal([]byte(value), &outputs)).To(Succeed())
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(outputs.Kubeconfig))
	Expect(err).NotTo(HaveOccurred())

	scheme := apis.NewScheme()

	k8sClient, err = client.New(restConfig, client.Options{
		Scheme: scheme,
	})

	ctx, ca := context.WithCancel(context.Background())
	DeferCleanup(ca)

	internalPorts, err := PortForward(ctx, types.NamespacedName{
		Namespace: "opni",
		Name:      "opni-internal",
	}, []string{
		"11090",
	}, restConfig, scheme)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(internalPorts)).To(Equal(1))

	Expect(testEnv.Start(
		test.WithEnableGateway(false),
		test.WithDefaultAgentOpts(
			test.WithRemoteGatewayAddress(outputs.GatewayURL+":9090"),
		),
	)).To(Succeed())

	mgmtClient, err = clients.NewManagementClient(ctx,
		clients.WithAddress(fmt.Sprintf("127.0.0.1:%d", internalPorts[0].Local)),
	)
	Expect(err).NotTo(HaveOccurred())

	adminClient, err = clients.FromExtension(ctx, mgmtClient, "CortexAdmin", cortexadmin.NewCortexAdminClient)
	Expect(err).NotTo(HaveOccurred())

	cortexOpsClient, err = clients.FromExtension(ctx, mgmtClient, "CortexOps", cortexops.NewCortexOpsClient)
	Expect(err).NotTo(HaveOccurred())

	loggingAdminClient, err = clients.FromExtension(ctx, mgmtClient, "LoggingAdmin", loggingadmin.NewLoggingAdminClient)
	Expect(err).NotTo(HaveOccurred())

	startStatus, err := cortexOpsClient.GetClusterStatus(ctx, &emptypb.Empty{})
	Expect(err).NotTo(HaveOccurred())
	if startStatus.State != cortexops.InstallState_NotInstalled {
		_, err = cortexOpsClient.UninstallCluster(ctx, &emptypb.Empty{})
		Expect(err).To(Succeed())

		Eventually(func() error {
			installStatus, err := cortexOpsClient.GetClusterStatus(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			if installStatus.State != cortexops.InstallState_NotInstalled {
				return fmt.Errorf("cortex is still installed")
			}
			return nil
		}, 90*time.Second, 5*time.Second).Should(Succeed())
	}
})

func unwrapOutputs(outputMap auto.OutputMap) map[string]any {
	outputs := make(map[string]any)
	for k, v := range outputMap {
		outputs[k] = v.Value
	}
	return outputs
}
