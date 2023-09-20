package metrics_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
	_ "github.com/rancher/opni/plugins/metrics/test"
)

func TestMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	SetDefaultEventuallyTimeout(5 * time.Second)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}
