package metrics_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/samber/lo"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/util/waitctx"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

const testNamespace = "test-ns"

// blockingHttpHandler is only here to keep a remote reader connection open to keep it running indefinitely
type blockingHttpHandler struct {
}

func (h blockingHttpHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/block":
		// select {} will block forever without using CPU.
		select {}
	case "/large":
		uncompressed, err := proto.Marshal(&prompb.ReadResponse{
			Results: []*prompb.QueryResult{
				{
					Timeseries: []*prompb.TimeSeries{
						{
							Labels: []prompb.Label{
								{
									Name:  "__name__",
									Value: "test_metric",
								},
							},
							// Samples: lo.Map(make([]prompb.Sample, 4194304), func(sample prompb.Sample, i int) prompb.Sample {
							Samples: lo.Map(make([]prompb.Sample, 65536), func(sample prompb.Sample, i int) prompb.Sample {
								sample.Timestamp = time.Now().UnixMilli()
								return sample
							}),
						},
					},
				},
			},
		})
		if err != nil {
			panic(err)
		}

		compressed := snappy.Encode(nil, uncompressed)

		_, err = w.Write(compressed)
		if err != nil {
			panic(err)
		}
	case "/health":
	default:
		panic(fmt.Sprintf("unsupported endpoint: %s", request.URL.Path))
	}
}

var _ = Describe("Remote Read Import", Ordered, Label("integration", "slow"), func() {
	ctx := context.Background()
	agentId := "import-agent"

	blockingTarget := &remoteread.Target{
		Meta: &remoteread.TargetMeta{
			Name:      "test-block",
			ClusterId: agentId,
		},
		Spec: &remoteread.TargetSpec{
			Endpoint: "",
		},
	}

	largeTarget := &remoteread.Target{
		Meta: &remoteread.TargetMeta{
			Name:      "test-large",
			ClusterId: agentId,
		},
		Spec: &remoteread.TargetSpec{
			Endpoint: "",
		},
	}

	query := &remoteread.Query{
		StartTimestamp: &timestamppb.Timestamp{
			// we only want this to run 1-3 times to avoid waiting for several large packets with backoffs due to ingest rate limiting, so we set the start time to be 2 * the read query step
			Seconds: int64(time.Now().Unix()) - int64(time.Minute.Seconds()*2),
		},
		EndTimestamp: &timestamppb.Timestamp{
			Seconds: time.Now().Unix(),
		},
		Matchers: []*remoteread.LabelMatcher{
			{
				Type:  remoteread.LabelMatcher_RegexEqual,
				Name:  "__name__",
				Value: ".+",
			},
		},
	}

	var env *test.Environment
	var importClient remoteread.RemoteReadGatewayClient

	BeforeAll(func() {
		Expect(os.Setenv("POD_NAMESPACE", "default")).ToNot(HaveOccurred())

		By("starting test environment")
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		By("adding an agent")
		managementClient := env.NewManagementClient()
		token, err := managementClient.CreateBootstrapToken(ctx, &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).ToNot(HaveOccurred())

		certInfo, err := managementClient.CertsInfo(ctx, &emptypb.Empty{})
		Expect(err).ToNot(HaveOccurred())

		_, errC := env.StartAgent(agentId, token, []string{certInfo.Chain[len(certInfo.Chain)-1].Fingerprint})
		Eventually(errC).Should(Receive(BeNil()))

		cortexCtx := waitctx.Background()
		env.StartCortex(cortexCtx)

		By("adding prometheus resources to k8s")
		k8sctx, ca := context.WithCancel(waitctx.Background())
		s := scheme.Scheme
		monitoringv1.AddToScheme(s)

		config, _, err := testk8s.StartK8s(k8sctx, []string{
			"../../../config/crd/prometheus",
		}, s)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			ca()
			waitctx.Wait(k8sctx)
		})

		kubeClient, err := kubernetes.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred())

		_, err = kubeClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: testNamespace,
			},
			Spec: corev1.NamespaceSpec{},
		}, apimetav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		promClient, err := monitoringclient.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred())

		_, err = promClient.MonitoringV1().Prometheuses("default").Create(context.Background(), &monitoringv1.Prometheus{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: "test-prometheus",
			},
			Spec: monitoringv1.PrometheusSpec{
				CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
					ExternalURL: "external-url.domain",
				},
			},
		}, apimetav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = promClient.MonitoringV1().Prometheuses(testNamespace).Create(context.Background(), &monitoringv1.Prometheus{
			ObjectMeta: apimetav1.ObjectMeta{
				Name: "test-prometheus",
			},
			Spec: monitoringv1.PrometheusSpec{
				CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
					ExternalURL: "external-url.domain",
				},
			},
		}, apimetav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("creating import client")
		importClient = remoteread.NewRemoteReadGatewayClient(env.ManagementClientConn())

		By("adding a remote read server")
		addr := fmt.Sprintf("127.0.0.1:%d", freeport.GetFreePort())

		blockingTarget.Spec.Endpoint = "http://" + addr + "/block"
		largeTarget.Spec.Endpoint = "http://" + addr + "/large"

		server := http.Server{
			Addr:    addr,
			Handler: blockingHttpHandler{},
		}

		go func() {
			server.ListenAndServe()
		}()
		// Shutdown will block indefinitely due to the server's blockingHttpHandler
		DeferCleanup(server.Close)

		// wait for http server to be up
		Eventually(func() error {
			_, err := (&http.Client{}).Get("http://" + addr + "/health")
			return err
		}).WithContext(ctx).Should(Not(HaveOccurred()))
	})

	When("add targets", func() {
		It("should succeed", func() {
			_, err := importClient.AddTarget(ctx, &remoteread.TargetAddRequest{
				Target:    blockingTarget,
				ClusterId: blockingTarget.Meta.ClusterId,
			})
			Expect(err).ToNot(HaveOccurred())

			_, err = importClient.AddTarget(ctx, &remoteread.TargetAddRequest{
				Target:    largeTarget,
				ClusterId: largeTarget.Meta.ClusterId,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on duplicate targets", func() {
			_, err := importClient.AddTarget(ctx, &remoteread.TargetAddRequest{
				Target:    blockingTarget,
				ClusterId: blockingTarget.Meta.ClusterId,
			})
			Expect(err).To(HaveOccurred())
		})
	})

	When("starting targets", func() {
		It("should succeed", func() {
			_, err := importClient.Start(ctx, &remoteread.StartReadRequest{
				Target: blockingTarget,
				Query:  query,
			})
			Expect(err).ToNot(HaveOccurred())

			status, err := importClient.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
				Meta: blockingTarget.Meta,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(status.State).To(Equal(remoteread.TargetState_Running))

			_, err = importClient.Start(ctx, &remoteread.StartReadRequest{
				Target: largeTarget,
				Query:  query,
			})
			Expect(err).ToNot(HaveOccurred())

			status, err = importClient.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
				Meta: largeTarget.Meta,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(status.State).To(Equal(remoteread.TargetState_Running))
		})
	})

	When("target is running", func() {
		It("should fail to start a running target", func() {
			_, err := importClient.Start(ctx, &remoteread.StartReadRequest{
				Target: blockingTarget,
				Query:  query,
			})
			Expect(err).To(HaveOccurred())
		})

		It("should fail to edit a target", func() {
			_, err := importClient.EditTarget(ctx, &remoteread.TargetEditRequest{
				Meta: blockingTarget.Meta,
				TargetDiff: &remoteread.TargetDiff{
					Name: "new-name",
				},
			})
			Expect(err).To(HaveOccurred())
		})

		It("should fail to delete a target", func() {
			_, err := importClient.RemoveTarget(ctx, &remoteread.TargetRemoveRequest{
				Meta: blockingTarget.Meta,
			})
			Expect(err).To(HaveOccurred())
		})

		It("should be able to send large data", func() {
			Eventually(func() (remoteread.TargetState, error) {
				status, err := importClient.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
					Meta: largeTarget.Meta,
				})

				return status.State, err
			}, "15s").Should(Equal(remoteread.TargetState_Completed))
		})
	})
})
