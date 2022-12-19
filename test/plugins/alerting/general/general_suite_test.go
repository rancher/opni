package general_test

import (
	"testing"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
)

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

var env *test.Environment

var alertingConditionClient alertingv1.AlertConditionsClient
var alertingEndpointClient alertingv1.AlertEndpointsClient
var alertingLogClient log.AlertLogsClient
var alertingTriggerClient alertingv1.AlertingClient

var adminClient cortexadmin.CortexAdminClient
var agentPort int
var kubernetesTempMetricServerPort int
var kubernetesJobName string = "kubernetesMock"
var _ = BeforeSuite(func() {

})
