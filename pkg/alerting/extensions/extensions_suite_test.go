package extensions_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
)

func TestExtensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extensions Suite")
}

var env *test.Environment
var tmpConfigDir string

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../testbin/bin",
	}
	Expect(env).NotTo(BeNil())
	Expect(env.Start()).To(Succeed())
	DeferCleanup(env.Stop)
	tmpConfigDir = env.GenerateNewTempDirectory("alertmanager-config")
	err := os.MkdirAll(tmpConfigDir, 0755)
	Expect(err).NotTo(HaveOccurred())
	Expect(tmpConfigDir).NotTo(Equal(""))
})
