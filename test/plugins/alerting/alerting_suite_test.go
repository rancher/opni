package alerting_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
	_ "github.com/rancher/opni/plugins/alerting/test"
)

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

var env *test.Environment
var tmpConfigDir string

var _ = BeforeSuite(func() {
	testruntime.IfIntegration(func() {
		env = &test.Environment{}
		Expect(env).NotTo(BeNil())
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)
		tmpConfigDir = env.GenerateNewTempDirectory("alertmanager-config")
		Expect(tmpConfigDir).NotTo(Equal(""))
	})
})
