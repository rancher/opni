package routing_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
)

func TestRouting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Routing Suite")
}

var env *test.Environment
var tmpConfigDir string

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../../testbin/bin",
	}
	Expect(env).NotTo(BeNil())
	Expect(env.Start()).To(Succeed())
	DeferCleanup(env.Stop)
	tmpConfigDir = env.GenerateNewTempDirectory("alertmanager-config")
	Expect(tmpConfigDir).NotTo(Equal(""))
})
