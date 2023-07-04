package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/plugins/example/pkg/example"
	_ "github.com/rancher/opni/plugins/example/test"
	"google.golang.org/protobuf/types/known/emptypb"
)

func ExpectGracefulExamplePluginShutdown(env *test.Environment) {
	exampleClient := example.NewExampleAPIExtensionClient(env.ManagementClientConn())
	Eventually(func() error {
		if _, err := exampleClient.Ready(env.Context(), &emptypb.Empty{}); err != nil {
			return err
		}
		return nil
	}).Should(Succeed()) // prevents process from exiting before the extension is setup
	Expect(env.Stop()).To(Succeed())
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}
