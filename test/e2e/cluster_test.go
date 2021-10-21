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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/phayes/freeport"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	opensearchapiext "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/utils/pointer"
)

const (
	clusterCrName      = "test-opnicluster"
	clusterCrNamespace = "opnicluster-test"
)

func queryAnomalyCountWithExtendedClient(esClient *indices.ExtendedClient) (int, error) {
	response, err := esClient.Count(
		esClient.Count.WithIndex("logs"),
		esClient.Count.WithQuery(`anomaly_level:Anomaly AND is_control_plane_log:true`),
		// esClient.Count.WithQuery(`anomaly_level:Anomaly`),
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

var _ = Describe("OpniCluster E2E Test", Label("e2e"), func() {
	var (
		pretrained  v1beta1.PretrainedModel
		logadapter  v1beta1.LogAdapter
		opnicluster v1beta1.OpniCluster
		esClient    indices.ExtendedClient
	)
	When("creating a pretrained model", func() {
		It("should succeed", func() {
			pretrained = v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterCrName,
					Namespace: clusterCrNamespace,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.1.2.zip",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.6"),
						"minLogTokens":   intstr.FromInt(4),
						"isControlPlane": intstr.FromString("true"),
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &pretrained)).To(Succeed())
		})
	})
	When("creating a logadapter", func() {
		It("should succeed", func() {
			logadapter = v1beta1.LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterCrName,
				},
				Spec: v1beta1.LogAdapterSpec{
					Provider: v1beta1.LogProviderK3S,
					K3S: &v1beta1.K3SSpec{
						ContainerEngine: v1beta1.ContainerEngineOpenRC,
					},
					OpniCluster: v1beta1.OpniClusterNameSpec{
						Name:      clusterCrName,
						Namespace: clusterCrNamespace,
					},
				},
			}
			logadapter.Default()
			Expect(k8sClient.Create(context.Background(), &logadapter)).To(Succeed())
		})
	})
	When("creating an opnicluster", func() {
		It("should succeed", func() {
			opnicluster = v1beta1.OpniCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterCrName,
					Namespace: clusterCrNamespace,
				},
				Spec: v1beta1.OpniClusterSpec{
					Version:            "v0.2.0",
					DeployLogCollector: pointer.BoolPtr(true),
					Services: v1beta1.ServicesSpec{
						GPUController: v1beta1.GPUControllerServiceSpec{
							Enabled: pointer.BoolPtr(false),
						},
						Metrics: v1beta1.MetricsServiceSpec{
							Enabled: pointer.BoolPtr(false),
						},
						Inference: v1beta1.InferenceServiceSpec{
							PretrainedModels: []corev1.LocalObjectReference{
								{
									Name: clusterCrName,
								},
							},
						},
						Insights: v1beta1.InsightsServiceSpec{
							Enabled: pointer.BoolPtr(false),
						},
						UI: v1beta1.UIServiceSpec{
							Enabled: pointer.BoolPtr(false),
						},
					},
					Elastic: v1beta1.ElasticSpec{
						Version: "1.13.2",
					},
					S3: v1beta1.S3Spec{
						Internal: &v1beta1.InternalSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &opnicluster)).To(Succeed())
		})
		It("should become ready", func() {
			opnicluster := v1beta1.OpniCluster{}
			i := 0
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      clusterCrName,
					Namespace: clusterCrNamespace,
				}, &opnicluster)
				if err != nil {
					return err
				}
				if opnicluster.Status.State == "" {
					return errors.New("State not populated yet")
				}
				if opnicluster.Status.State != "Ready" {
					conditions := strings.Join(opnicluster.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				if opnicluster.Status.IndexState != "Ready" {
					conditions := strings.Join(opnicluster.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				if opnicluster.Status.LogCollectorState != "Ready" {
					conditions := strings.Join(opnicluster.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				Expect(opnicluster.Status.Conditions).To(BeEmpty(),
					"Expected no conditions if state is Ready")
				return nil
			}, 10*time.Minute, 500*time.Millisecond).Should(BeNil())
		})
	})
	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	portForwardPort, _ := freeport.GetFreePort()
	Specify("port-forward setup", func() {
		svc := &corev1.Service{}
		Expect(k8sClient.Get(context.Background(), types.NamespacedName{
			Namespace: clusterCrNamespace,
			Name:      "opni-es-client",
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
	Context("verify elasticsearch setup", func() {
		It("should be able to create elasticsearch client", func() {
			var err error

			By("fetching the password secret")
			secret := &corev1.Secret{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "opni-es-password",
				Namespace: clusterCrNamespace,
			}, secret)
			Expect(err).NotTo(HaveOccurred())
			password, ok := secret.Data["password"]
			Expect(ok).To(BeTrue())

			By("creating the client")
			elasticClient, err := opensearch.NewClient(opensearch.Config{
				Addresses:            []string{fmt.Sprintf("https://127.0.0.1:%d", portForwardPort)},
				Username:             "admin",
				Password:             string(password),
				UseResponseCheckOnly: true,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			})
			esClient = indices.ExtendedClient{
				Client: elasticClient,
				ISM:    &indices.ISMApi{Client: elasticClient},
			}
			Expect(err).NotTo(HaveOccurred())
		})
		Specify("ISM policies should be created", func() {
			for _, policy := range []string{
				"log-policy",
				"opni-drain-model-status-policy",
			} {
				Expect(func() bool {
					resp, err := esClient.ISM.GetISM(context.Background(), policy)
					if err != nil {
						fmt.Println(err)
						return true
					}
					defer resp.Body.Close()
					isError := resp.IsError()
					if isError {
						fmt.Println(policy, resp.Status())
					}
					return isError
				}()).To(BeFalse())
			}
		})
		Specify("index templates should exist", func() {
			for _, template := range []string{
				"logs_rollover_mapping",
				"opni-drain-model-status_rollover_mapping",
			} {
				Expect(func() bool {
					req := opensearchapi.IndicesGetIndexTemplateRequest{
						Name: []string{
							template,
						},
					}
					resp, err := req.Do(context.Background(), esClient)
					if err != nil {
						fmt.Println(err)
						return true
					}
					defer resp.Body.Close()
					isError := resp.IsError()
					if isError {
						fmt.Println(template, resp.Status())
					}
					return isError
				}()).Should(BeFalse())
			}
		})
		Specify("indices should be created", func() {
			for _, index := range []string{
				"logs-v0.1.3-000001",
				"opni-drain-model-status-v0.1.3-000001",
				"opni-normal-intervals",
				"opni-dashboard-version",
			} {
				Expect(func() bool {
					req := opensearchapi.CatIndicesRequest{
						Index: []string{
							index,
						},
						Format: "json",
					}
					resp, err := req.Do(context.Background(), esClient)
					if err != nil {
						fmt.Println(err)
						return true
					}
					defer resp.Body.Close()
					isError := resp.IsError()
					if isError {
						fmt.Println(index, resp.Status())
					}
					return isError
				}()).Should(BeFalse())
			}
		})
		Specify("kibana version document should be correct", func() {
			respDoc := &opensearchapiext.KibanaDocResponse{}
			Expect(func() bool {
				req := opensearchapi.GetRequest{
					Index:      "opni-dashboard-version",
					DocumentID: "latest",
				}
				resp, err := req.Do(context.Background(), esClient)
				if err != nil {
					fmt.Println(err)
					return true
				}
				defer resp.Body.Close()
				if resp.IsError() {
					return true
				}
				err = json.NewDecoder(resp.Body).Decode(respDoc)
				if err != nil {
					fmt.Println(err)
					return true
				}
				return false
			}()).To(BeFalse())
			Expect(respDoc.Source.DashboardVersion).To(Equal("v0.1.3"))
		})
	})
	Context("verify logs are being shipped to elasticsearch", func() {
		Specify("elasticsearch should contain logs", func() {
			Eventually(func() int {
				response, err := esClient.Count(esClient.Count.WithIndex("logs"))
				Expect(err).NotTo(HaveOccurred())
				countResp := CountResponse{}
				Expect(json.NewDecoder(response.Body).Decode(&countResp)).To(Succeed())
				return countResp.Count
			}, 5*time.Minute, 1*time.Second).Should(BeNumerically(">", 0))
		})
		Specify("anomaly count should increase when faults are injected", func() {
			By("sampling anomaly count (30s)")
			experiment := gmeasure.NewExperiment("fault injection")
			experiment.SampleValue("before", func(idx int) float64 {
				defer time.Sleep(500 * time.Millisecond)
				count, err := queryAnomalyCountWithExtendedClient(&esClient)
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
				count, err := queryAnomalyCountWithExtendedClient(&esClient)
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
			//Expect(after.FloatFor(gmeasure.StatMax)).Should(BeNumerically(">", 20))
		})
	})
	Specify("clean up port-forward", func() {
		close(stopCh)
	})
	Specify("delete resources", func() {
		k8sClient.Delete(context.Background(), &opnicluster)
		k8sClient.Delete(context.Background(), &logadapter)
		k8sClient.Delete(context.Background(), &pretrained)
		k8sClient.DeleteAllOf(context.Background(), &corev1.Pod{}, client.InNamespace("default"))
	})
})
