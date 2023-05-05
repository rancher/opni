package metrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
	_ "github.com/rancher/opni/plugins/metrics/test"
)

func TestMetrics(t *testing.T) {
	suiteConfig := types.NewDefaultSuiteConfig()
	suiteConfig.FocusFiles = []string{"runner_test.go"}
	suiteConfig.FocusStrings = []string{"target is stopped during push"}

	reporterConfig := types.NewDefaultReporterConfig()
	reporterConfig.Verbose = true

	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite", suiteConfig, reporterConfig)
}
