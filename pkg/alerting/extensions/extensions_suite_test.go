package extensions_test

import (
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"gopkg.in/yaml.v2"
)

func TestExtensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AlertManager Extensions Suite")
}

var env *test.Environment
var tmpConfigDir string
var embeddedJetstream nats.JetStreamContext
var alertmanagerPort int
var opniPort int
var configFilePath string

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../testbin/bin",
	}
	Expect(env).NotTo(BeNil())
	Expect(env.Start()).To(Succeed())
	nc, err := env.StartEmbeddedJetstream()
	Expect(err).NotTo(HaveOccurred())
	embeddedJetstream, err = nc.JetStream()
	Expect(err).NotTo(HaveOccurred())
	Expect(embeddedJetstream).NotTo(BeNil())

	DeferCleanup(env.Stop)
	tmpConfigDir = env.GenerateNewTempDirectory("embedded-server")
	Expect(tmpConfigDir).NotTo(Equal(""))
	err = os.MkdirAll(tmpConfigDir, 0755)
	Expect(err).NotTo(HaveOccurred())
	opniPort, err = freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())
	configFilePath = tmpConfigDir + "/alertmanager.yml"

	err = os.WriteFile(
		configFilePath,
		util.Must(yaml.Marshal(util.Must(routing.NewDefaultOpniRouting().BuildConfig()))),
		0755,
	)
	Expect(err).NotTo(HaveOccurred())
	webPort, alertmanagerCtxCa := env.StartEmbeddedAlertManager(env.Context(), configFilePath, opniPort)
	DeferCleanup(func() {
		alertmanagerCtxCa()
	})
	alertmanagerPort = webPort
})
