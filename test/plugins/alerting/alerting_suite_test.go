package alerting_test

import (
	"fmt"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

type InvalidInputs struct {
	req proto.Message
	err error
}

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

var env *test.Environment
var alertingClient alertingv1alpha.AlertingClient
var _ = BeforeSuite(func() {
	fmt.Println("Starting BeforeSuite...")
	alerting.AlertPath = "alerttestdata/logs"
	err := os.RemoveAll(alerting.AlertPath)
	Expect(err).To(BeNil())
	err = os.MkdirAll(alerting.AlertPath, 0777)
	Expect(err).To(BeNil())

	err = os.Setenv(alerting.LocalBackendEnvToggle, "true")
	Expect(err).To(Succeed())
	err = os.WriteFile(alerting.LocalAlertManagerPath, []byte(alerting.DefaultAlertManager), 0666)
	Expect(err).To(Succeed())

	// get all the integration endpoint test information

	// test environment references
	// setup managemet server & client
	env = &test.Environment{
		TestBin: "../../../testbin/bin",
	}
	Expect(env.Start()).To(Succeed())
	DeferCleanup(env.Stop)

	// alerting plugin
	alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	fmt.Println("Finished BeforeSuite...")
})
