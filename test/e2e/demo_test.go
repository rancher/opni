package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/apis/demo/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore float64 `json:"max_score"`
		Hits     []struct {
			Index  string  `json:"_index"`
			Type   string  `json:"_type"`
			ID     string  `json:"_id"`
			Score  float64 `json:"_score"`
			Source struct {
				Log                         string    `json:"log"`
				Time                        time.Time `json:"time"`
				TimeNanoseconds             int64     `json:"time_nanoseconds"`
				WindowDt                    int64     `json:"window_dt"`
				WindowStartTimeNs           int64     `json:"window_start_time_ns"`
				MaskedLog                   string    `json:"masked_log"`
				Timestamp                   time.Time `json:"timestamp"`
				IsControlPlaneLog           bool      `json:"is_control_plane_log"`
				KubernetesComponent         string    `json:"kubernetes_component"`
				AnomalyPredictedCount       float64   `json:"anomaly_predicted_count"`
				NulogAnomaly                bool      `json:"nulog_anomaly"`
				DrainAnomaly                bool      `json:"drain_anomaly"`
				NulogConfidence             float64   `json:"nulog_confidence"`
				DrainMatchedTemplateID      float64   `json:"drain_matched_template_id"`
				DrainMatchedTemplateSupport float64   `json:"drain_matched_template_support"`
				AnomalyLevel                string    `json:"anomaly_level"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type CountResponse struct {
	Count  int `json:"count"`
	Shards struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
}

const (
	demoCrName      = "test-opnidemo"
	demoCrNamespace = "opnidemo-test"
)

func queryAnomalyCount(esClient *elasticsearch.Client) (int, error) {
	response, err := esClient.Count(
		esClient.Count.WithIndex("logs"),
		// esClient.Count.WithQuery(`anomaly_level:Anomalous AND is_control_plane_log:true`),
		esClient.Count.WithQuery(`anomaly_level:Suspicious`),
	)
	if err != nil {
		return 0, err
	}
	countResp := CountResponse{}
	if err := json.NewDecoder(response.Body).Decode(&countResp); err != nil {
		return 0, err
	}
	return countResp.Count, nil
}

var _ = Describe("OpniDemo E2E", func() {
	When("creating an opnidemo", func() {
		It("should succeed", func() {
			demo := v1alpha1.OpniDemo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      demoCrName,
					Namespace: demoCrNamespace,
				},
				Spec: v1alpha1.OpniDemoSpec{
					Components: v1alpha1.ComponentsSpec{
						Infra: v1alpha1.InfraStack{
							DeployHelmController: false,
							DeployNvidiaPlugin:   false,
						},
						Opni: v1alpha1.OpniStack{
							DeployGpuServices: false,
							Minio: v1alpha1.ChartOptions{
								Enabled: true,
							},
							Nats: v1alpha1.ChartOptions{
								Enabled: true,
							},
							Elastic: v1alpha1.ChartOptions{
								Enabled: true,
							},
							RancherLogging: v1alpha1.ChartOptions{
								Enabled: true,
								Set: map[string]intstr.IntOrString{
									"additionalLoggingSources.k3s.enabled":                    intstr.FromString("true"),
									"additionalLoggingSources.k3s.container_engine":           intstr.FromString("openrc"),
									"elasticsearch.master.readinessProbe.initialDelaySeconds": intstr.FromString("5"),
									"elasticsearch.data.readinessProbe.initialDelaySeconds":   intstr.FromString("5"),
									"elasticsearch.client.readinessProbe.initialDelaySeconds": intstr.FromString("5"),
									"kibana.readinessProbe":                                   intstr.FromString("initialDelaySeconds: 5"),
								},
							},
						},
					},
					MinioAccessKey:         "testAccessKey",
					MinioSecretKey:         "testSecretKey",
					MinioVersion:           "8.0.10",
					NatsVersion:            "2.2.1",
					NatsPassword:           "password",
					NatsReplicas:           1,
					NatsMaxPayload:         10485760,
					NvidiaVersion:          "1.0.0-beta6",
					ElasticsearchUser:      "admin",
					ElasticsearchPassword:  "admin",
					NulogServiceCPURequest: "1",
				},
			}
			Expect(k8sClient.Create(context.Background(), &demo)).To(Succeed())
		})
		It("should become ready", func() {
			demo := v1alpha1.OpniDemo{}
			i := 0
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      demoCrName,
					Namespace: demoCrNamespace,
				}, &demo)
				if err != nil {
					return err
				}
				if demo.Status.State == "" {
					return errors.New("State not populated yet")
				}
				if demo.Status.State != "Ready" {
					conditions := strings.Join(demo.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				Expect(demo.Status.Conditions).To(BeEmpty(),
					"Expected no conditions if state is Ready")
				return nil
			}, 10*time.Minute, 500*time.Millisecond).Should(BeNil())
		})
	})
	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	portForwardPort, _ := freeport.GetFreePort()
	Context("verifying logs are being shipped to elasticsearch", func() {
		Specify("port-forward setup", func() {
			svc := &corev1.Service{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{
				Namespace: demoCrNamespace,
				Name:      "opendistro-es-client-service",
			}, svc)).To(Succeed())

			pods := &corev1.PodList{}
			Eventually(func() bool {
				k8sClient.List(context.Background(), pods, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set(svc.Spec.Selector)),
				})
				return len(pods.Items) > 0
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			for _, pod := range pods.Items {
				transport, upgrader, err := spdy.RoundTripperFor(restConfig)
				Expect(err).NotTo(HaveOccurred())
				dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost,
					&url.URL{
						Scheme: "https",
						Path: fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward",
							pod.Namespace, pod.Name),
						Host: strings.TrimLeft(restConfig.Host, "htps:/"),
					})
				forwarder, err := portforward.New(dialer, []string{
					fmt.Sprintf("%d:%d", portForwardPort, 9200),
				}, stopCh, readyCh, os.Stdout, os.Stderr)
				Expect(err).NotTo(HaveOccurred())
				go func() {
					defer GinkgoRecover()
					Expect(forwarder.ForwardPorts()).To(Succeed())
				}()
				Eventually(readyCh).Should(BeClosed())
				break
			}
		})
		var esClient *elasticsearch.Client
		Specify("elasticsearch should contain logs", func() {
			var err error
			esClient, err = elasticsearch.NewClient(elasticsearch.Config{
				Addresses: []string{fmt.Sprintf("https://127.0.0.1:%d", portForwardPort)},
				Username:  "admin",
				Password:  "admin",
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				response, err := esClient.Count(esClient.Count.WithIndex("logs"))
				Expect(err).NotTo(HaveOccurred())
				countResp := CountResponse{}
				Expect(json.NewDecoder(response.Body).Decode(&countResp)).To(Succeed())
				return countResp.Count
			}, 5*time.Minute, 1*time.Second).Should(BeNumerically(">", 0))
		})
		XSpecify("anomaly count should increase when faults are injected", func() {
			By("sampling anomaly count (30s)")
			experiment := gmeasure.NewExperiment("fault injection")
			experiment.SampleValue("before", func(idx int) float64 {
				defer time.Sleep(500 * time.Millisecond)
				count, err := queryAnomalyCount(esClient)
				Expect(err).NotTo(HaveOccurred())
				return float64(count)
			}, gmeasure.SamplingConfig{
				Duration: 30 * time.Second,
			})
			By("injecting faults")
			// Create 10 pods with nonexistent images
			// Create 10 pods that will exit with non-zero exit codes
			for i := 0; i < 10; i++ {
				Expect(k8sClient.Create(context.Background(), &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-%d", "opni-fault-injection-no-image", i),
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "test",
								Image:           "this-image-does-not-exist",
								Command:         []string{"/test"},
								ImagePullPolicy: corev1.PullAlways,
							},
						},
					},
				})).To(Succeed())
			}
			for i := 0; i < 10; i++ {
				Expect(k8sClient.Create(context.Background(), &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-%d", "opni-fault-injection-fail", i),
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    "test",
								Image:   "busybox",
								Command: []string{"/bin/false"},
							},
						},
					},
				})).To(Succeed())
			}
			By("sampling anomaly count after fault injection (30s)")
			experiment.SampleValue("after", func(idx int) float64 {
				defer time.Sleep(500 * time.Millisecond)
				count, err := queryAnomalyCount(esClient)
				Expect(err).NotTo(HaveOccurred())
				return float64(count)
			}, gmeasure.SamplingConfig{
				Duration: 30 * time.Second,
			})
			before := experiment.GetStats("before")
			after := experiment.GetStats("after")
			r1 := gmeasure.RankStats(gmeasure.HigherMaxIsBetter, before, after)
			r2 := gmeasure.RankStats(gmeasure.HigherMeanIsBetter, before, after)
			r3 := gmeasure.RankStats(gmeasure.HigherMedianIsBetter, before, after)
			fmt.Printf("Anomaly Count (before fault injection): %s\n", before.String())
			fmt.Printf("Anomaly Count  (after fault injection): %s\n", after.String())
			Expect(r1.Winner()).To(Equal(after))
			Expect(r2.Winner()).To(Equal(after))
			Expect(r3.Winner()).To(Equal(after))
			Expect(after.FloatFor(gmeasure.StatMax)).Should(BeNumerically(">", 20))
		})
		Specify("clean up port-forward", func() {
			close(stopCh)
		})
	})
})
