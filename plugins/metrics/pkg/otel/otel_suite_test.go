package otel_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
)

var env *test.Environment

func TestOtel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Otel Suite")
}

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../../testbin/bin",
	}
	Expect(env).NotTo(BeNil())
})
