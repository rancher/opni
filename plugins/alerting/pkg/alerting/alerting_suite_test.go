package alerting_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
	_ "github.com/rancher/opni/plugins/alerting/test"
)

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}
