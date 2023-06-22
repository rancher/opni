package plugins_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
	_ "github.com/rancher/opni/plugins/alerting/test"
	_ "github.com/rancher/opni/plugins/metrics/test"
	_ "github.com/rancher/opni/plugins/slo/test"
)

func TestSloPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plugins Suite")
}
