package plugins_test

import (
	cortexadmin "github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type TestSeriesMetrics struct {
	input  *cortexadmin.SeriesRequest
	output *cortexadmin.SeriesInfoList
}

type TestMetricLabelSet struct {
	input  *cortexadmin.LabelRequest
	output *cortexadmin.MetricLabels
}

func TestCortexadmin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cortexadmin Suite")
}
