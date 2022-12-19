package conditions_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
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

type mockPod struct {
	podName   string
	namespace string
	phase     string
	uid       string
}

var _ = BeforeSuite(func() {

})

func setMockKubernetesPodState(kubePort int, pod *mockPod) {
	queryUrl := fmt.Sprintf("http://localhost:%d/set", kubePort)
	client := &http.Client{
		Transport: &http.Transport{},
	}
	req, err := http.NewRequest("GET", queryUrl, nil)
	if err != nil {
		panic(err)
	}
	values := url.Values{}
	values.Set("obj", "pod")
	values.Set("name", pod.podName)
	values.Set("namespace", pod.namespace)
	values.Set("phase", pod.phase)
	values.Set("uid", pod.uid)
	req.URL.RawQuery = values.Encode()
	go func() {
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			panic(fmt.Sprintf("kube metrics prometheus collector hit an error %d", resp.StatusCode))
		}
	}()
}

func getRawMetrics(kubePort int) io.ReadCloser {
	queryUrl := fmt.Sprintf("http://localhost:%d/metrics", kubePort)
	client := &http.Client{
		Transport: &http.Transport{},
	}
	req, err := http.NewRequest("GET", queryUrl, nil)
	if err != nil {
		panic(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("kube metrics prometheus collector hit an error %d", resp.StatusCode))
	}
	return resp.Body
}
