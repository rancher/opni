package alerting_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
)

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

var env *test.Environment
var tmpConfigDir string

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../testbin/bin",
	}
	Expect(env).NotTo(BeNil())
	Expect(env.Start(
		test.WithEnableAlertingClusterDriver(true),
	)).To(Succeed())
	DeferCleanup(env.Stop)
	tmpConfigDir = env.GenerateNewTempDirectory("alertmanager-config")
	Expect(tmpConfigDir).NotTo(Equal(""))
})
