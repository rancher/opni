package configv1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
)

func TestConfigV1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config V1 Suite")
}
