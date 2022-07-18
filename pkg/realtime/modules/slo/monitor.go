package slo

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"go.uber.org/zap"
)

type monitor struct {
	slo               *slo.SLOData
	mgmtClient        managementv1.ManagementClient
	cortexAdminClient cortexadmin.CortexAdminClient
	sloClient         slo.SLOClient
	logger            *zap.SugaredLogger

	sloStatus *prometheus.GaugeVec
}

func (t *monitor) InitMetrics() []prometheus.Collector {
	// TODO(yingbei): add/update metrics here as needed
	t.sloStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "opni",
		Subsystem: "rt",
		Name:      "slo_status",
		Help:      "SLO status",
	}, []string{
		"metric_name",
		"target",
		"desc",
	})
	return []prometheus.Collector{
		t.sloStatus,
	}
}

type sloState struct {
	Time int64 `json:"time"`
}

type queryResponse struct {
	data struct {
		result []model.Sample `json:"result"`
	} `json:"data"`
}

func (t *monitor) Run(ctx context.Context, state io.ReadWriter) {
	// TODO(yingbei): implement

	s := sloState{
		Time: time.Now().Unix(),
	}

	json.NewEncoder(state).Encode(s)
	json.NewDecoder(state).Decode(&s)

	// example code:
	for {
		// get current value of the SLI metric
		// query range: say last 1 min, it needs a start time, end time, step

		// var query = "opni_rt_slo_status"
		// query = t.slo.Name
		// resp, err := t.cortexAdminClient.QueryRange(context.Background(), &cortexadmin.QueryRequest{
		// 	Tenants: []string{t.slo.ClusterId},
		// 	Query:   query,
		// })
		// if err != nil {

		// }
		// qr := queryResponse{}
		// json.Unmarshal(resp.Data, &qr)
		// var sliMetric = qr.data.result[0].value
		// var sloMetric = 0.0
		// // apply formulars, for now we can suport sum, average, percentile(99%)?
		// if t.slo.FormulaId == "1" {
		// 	sloMetric = sum(sliMetric)
		// } else {
		// 	sloMetric = mean(sliMetric)
		// }

		// //comapre to targets, assuming do smaller( < ) , and calculate error budget accordingly
		// //TODO: slo need a start date or created date
		// for _, threshold := range t.slo.Targets { // Timewindow = [[7days, 99.5%], [300days, 99.95%]], Targets = [200ms, 200ms]
		// 	var timeWindow, percentage = t.slo.TimeWindow[0], t.slo.TimeWindow[1]
		// 	var totalErrorBudget = timeWindow * (1 - percentage) * 24 * 60 * 60 // seconds
		// 	var remainingBudget float64
		// 	if sloMetric < threshold {
		// 		// a Normal case
		// 	} else {
		// 		// an Error case that reduce available error budget
		// 		t.sloStatus.errorBudget -= 60 // seconds, value depends on the scrape interval
		// 		remainingBudget = t.sloStatus.errorBudget

		// 		// TODO: Warnings depends on remainingBudget
		// 		// hard warning threshold, optional now
		// 		if remainingBudget < warningThreshold {
		// 			// do somehthing warning
		// 		}
		// 		//burning rate
		// 		// if remainingBudget/totalErrorBudget < (currentTime-startTime)/timeWindow {

		// 		// }
		// 	}
		// }

		// t.sloStatus.With(prometheus.Labels{
		// 	"metric_name": "foo",
		// 	"target":      strconv.FormatUint(t.slo.Targets[0].ValueX100, 10),
		// 	"desc":        "todo",
		// }).Set(0)

		// time.Sleep(1)
	}
}
