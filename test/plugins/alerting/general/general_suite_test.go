package general_test

import (
	"testing"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/condition"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/endpoint"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/trigger"
)

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

var env *test.Environment

var alertingConditionClient condition.AlertConditionsClient
var alertingEndpointClient endpoint.AlertEndpointsClient
var alertingLogClient log.AlertLogsClient
var alertingTriggerClient trigger.AlertingClient

var adminClient cortexadmin.CortexAdminClient
var agentPort int
var kubernetesTempMetricServerPort int
var kubernetesJobName string = "kubernetesMock"
var _ = BeforeSuite(func() {

})
