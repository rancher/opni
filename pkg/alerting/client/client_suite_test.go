package client_test

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

var (
	env  *test.Environment
	cl   client.AlertingClient
	clHA client.AlertingClient
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../testbin/bin",
	}
	Expect(env.Start()).To(Succeed())

	opniPort := freeport.GetFreePort()

	// set up a config file
	router := routing.NewDefaultOpniRoutingWithOverrideHook(fmt.Sprintf("http://localhost:%d%s", opniPort, shared.AlertingDefaultHookName))
	cfg, err := router.BuildConfig()
	Expect(err).To(Succeed())
	dir := env.GenerateNewTempDirectory("alertmanager_client")
	Expect(os.MkdirAll(dir, 0755)).To(Succeed())
	file, err := os.Create(fmt.Sprintf("%s/alertmanager.yaml", dir))
	Expect(err).To(Succeed())
	err = yaml.NewEncoder(file).Encode(cfg)
	Expect(err).To(Succeed())

	// start alertmanager
	ports := env.StartEmbeddedAlertManager(env.Context(), file.Name(), lo.ToPtr(opniPort))
	clA := client.NewClient(
		nil,
		fmt.Sprintf("http://localhost:%d", ports.ApiPort),
		fmt.Sprintf("http://localhost:%d", ports.EmbeddedPort),
	)
	cl = clA

	msgPort := freeport.GetFreePort()
	haRouter := routing.NewDefaultOpniRoutingWithOverrideHook(fmt.Sprintf("http://localhost:%d%s", msgPort, shared.AlertingDefaultHookName))
	haCfg, err := haRouter.BuildConfig()
	Expect(err).To(Succeed())
	haFile, err := os.Create(fmt.Sprintf("%s/ha_alertmanager.yaml", dir))
	Expect(err).To(Succeed())
	err = yaml.NewEncoder(haFile).Encode(haCfg)
	Expect(err).To(Succeed())
	haPorts := env.StartEmbeddedAlertManager(env.Context(), haFile.Name(), lo.ToPtr(msgPort))

	replica1 := env.StartEmbeddedAlertManager(
		env.Context(),
		haFile.Name(),
		lo.ToPtr(freeport.GetFreePort()),
		fmt.Sprintf("127.0.0.1:%d", haPorts.ClusterPort),
	)
	replica2 := env.StartEmbeddedAlertManager(
		env.Context(),
		haFile.Name(),
		lo.ToPtr(freeport.GetFreePort()),
		fmt.Sprintf("127.0.0.1:%d", haPorts.ClusterPort),
		fmt.Sprintf("127.0.0.1:%d", replica1.ClusterPort),
	)

	clHA = client.NewClient(
		nil,
		fmt.Sprintf("http://localhost:%d", ports.ApiPort),
		fmt.Sprintf("http://localhost:%d", ports.EmbeddedPort),
	)

	clHA.MemberlistClient().SetKnownPeers([]client.AlertingPeer{
		{
			ApiAddress:      fmt.Sprintf("http://localhost:%d", haPorts.ApiPort),
			EmbeddedAddress: fmt.Sprintf("http://localhost:%d", haPorts.EmbeddedPort),
		},
		{
			ApiAddress:      fmt.Sprintf("http://localhost:%d", replica2.ApiPort),
			EmbeddedAddress: fmt.Sprintf("http://localhost:%d", replica2.EmbeddedPort),
		},
		{
			ApiAddress:      fmt.Sprintf("http://localhost:%d", replica1.ApiPort),
			EmbeddedAddress: fmt.Sprintf("http://localhost:%d", replica1.EmbeddedPort),
		},
	})

	DeferCleanup(func() {
		Expect(env.Stop()).To(Succeed())
	})
})
