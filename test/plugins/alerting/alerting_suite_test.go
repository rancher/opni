package alerting_test

import (
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = BeforeSuite(func() {
	os.Setenv(alerting.LocalBackendEnvToggle, "true")
	err := os.WriteFile(alerting.LocalAlertManagerPath, []byte(alerting.DefaultAlertManager), 0644)
	Expect(err).To(Succeed())

})
