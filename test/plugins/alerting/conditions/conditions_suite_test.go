package conditions_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

func defaultConfig() (bytes.Buffer, error) {
	templateToFill := shared.DefaultAlertManager
	var b bytes.Buffer
	err := templateToFill.Execute(&b, shared.DefaultAlertManagerInfo{
		CortexHandlerName: "web.hook",
		CortexHandlerURL:  "http://127.0.0.1:5001/",
	})
	if err != nil {
		panic(err)
	}

	return b, err
}

type InvalidInputs struct {
	req proto.Message
	err error
}
type TestSuiteState struct {
	numAlertConditions int
	numLogs            int
	mockPods           []*mockPod
}

type mockPod struct {
	podName   string
	namespace string
	phase     string
	uid       string
}

func sampleRandomKubeState() string {
	r := rand.Intn(len(metrics.KubeStates))
	return metrics.KubeStates[r]
}

func newRandomMockPod() *mockPod {
	podName := test.RandomName(time.Now().UnixNano())
	namespace := test.RandomName(time.Now().UnixNano())
	phase := sampleRandomKubeState()
	uid := uuid.New().String()
	return &mockPod{
		podName:   podName,
		namespace: namespace,
		phase:     phase,
		uid:       uid,
	}
}

var env *test.Environment
var alertingClient alertingv1alpha.AlertingClient
var adminClient cortexadmin.CortexAdminClient
var agentPort int
var kubernetesTempMetricServerPort int
var kubernetesJobName string = "kubernetesMock"
var curTestState TestSuiteState

var _ = BeforeSuite(func() {
	fmt.Println("Starting BeforeSuite...")
	alerting.AlertPath = "alerttestdata/logs"
	err := os.RemoveAll(alerting.AlertPath)
	Expect(err).To(BeNil())
	err = os.MkdirAll(alerting.AlertPath, 0777)
	Expect(err).To(BeNil())

	err = os.Setenv(shared.LocalBackendEnvToggle, "true")
	Expect(err).To(Succeed())
	defaultCfg, err := defaultConfig()
	Expect(err).NotTo(HaveOccurred())
	err = os.WriteFile(shared.LocalAlertManagerPath, defaultCfg.Bytes(), 0666)
	Expect(err).To(Succeed())

	// get all the integration endpoint test information

	// test environment references
	// setup managemet server & client
	env = &test.Environment{
		TestBin: "../../../../testbin/bin",
	}
	Expect(env.Start()).To(Succeed())
	DeferCleanup(env.Stop)

	// setup a kubernetes metric mock
	kubernetesTempMetricServerPort = env.StartMockKubernetesMetricServer(context.Background())
	fmt.Printf("Mock kubernetes metrics server started on port %d\n", kubernetesTempMetricServerPort)
	for i := 0; i < 10; i++ {
		pod := newRandomMockPod()
		setMockKubernetesPodState(kubernetesTempMetricServerPort, pod)
		curTestState.mockPods = append(curTestState.mockPods, pod)
	}

	// set up a downstream
	client := env.NewManagementClient()
	token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
		Ttl: durationpb.New(time.Hour),
	})
	Expect(err).NotTo(HaveOccurred())
	info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
	Expect(err).NotTo(HaveOccurred())
	p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
	agentPort = env.StartPrometheus(p, test.NewOverridePrometheusConfig(
		"alerting/prometheus/config.yaml",
		[]test.PrometheusJob{
			{
				JobName:    kubernetesJobName,
				ScrapePort: kubernetesTempMetricServerPort,
			},
		}),
	)
	fmt.Println("agent port : ", agentPort)
	adminClient = env.NewCortexAdminClient()
	// alerting plugin
	alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	Eventually(func() error {
		stats, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
		if err != nil {
			return err
		}
		for _, item := range stats.Items {
			if item.UserID == "agent" {
				if item.NumSeries > 0 {
					return nil
				}
			}
		}
		return fmt.Errorf("waiting for metric data to be stored in cortex")
	}, 30*time.Second, 1*time.Second).Should(Succeed())
	fmt.Println("Finished BeforeSuite...")
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
