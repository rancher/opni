package metrics_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	apimetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const testNamespace = "test-ns"

// blockingHttpHandler is only here to keep a remote reader connection open to keep it running indefinitely
type blockingHttpHandler struct {
}

func (h blockingHttpHandler) ServeHTTP(_ http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/block":
		select {}
	default:
	}
}

var _ = Describe("Remote Read Import", Ordered, Label("integration", "slow"), func() {
	ctx := context.Background()
	agentId := "import-agent"

	target := &remoteread.Target{
		Meta: &remoteread.TargetMeta{
			Name:      "test",
			ClusterId: agentId,
		},
		Spec: &remoteread.TargetSpec{
			Endpoint: "",
		},
		Status: nil,
	}

	query := &remoteread.Query{
		StartTimestamp: &timestamppb.Timestamp{
			Seconds: time.Now().Unix() - int64(time.Hour.Seconds()),
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

		By("adding prometheus resources to k8s")
		k8sctx, ca := context.WithCancel(waitctx.Background())
		config, _, err := testk8s.StartK8s(k8sctx, []string{
			"../../../config/crd/prometheus",
		})
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

		By("adding a dummy remote read server")
		port := freeport.GetFreePort()

		addr := fmt.Sprintf("127.0.0.1:%d", port)
		target.Spec.Endpoint = "http://" + addr + "/block"

		server := http.Server{
			Addr:    addr,
			Handler: blockingHttpHandler{},
		}

		go func() {
			server.ListenAndServe()
		}()
		// Shutdown will block indefinitely due to the server's blockingHttpHandler
		DeferCleanup(server.Close)

		// wait fo rhttp server to be up
		Eventually(func() error {
			_, err := (&http.Client{}).Get("http://" + addr)
			return err
		}).WithContext(ctx).Should(Not(HaveOccurred()))
	})

	When("add targets", func() {
		It("should succeed", func() {
			_, err := importClient.AddTarget(ctx, &remoteread.TargetAddRequest{
				Target:    target,
				ClusterId: target.Meta.ClusterId,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on duplicate targets", func() {
			_, err := importClient.AddTarget(ctx, &remoteread.TargetAddRequest{
				Target:    target,
				ClusterId: target.Meta.ClusterId,
			})
			Expect(err).To(HaveOccurred())
		})
	})

	When("starting a target", func() {
		It("should succeed", func() {
			_, err := importClient.Start(ctx, &remoteread.StartReadRequest{
				Target: target,
				Query:  query,
			})
			Expect(err).ToNot(HaveOccurred())

			status, err := importClient.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
				Meta: target.Meta,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(status.State).To(Equal(remoteread.TargetState_Running))
		})
	})

	When("target is running", func() {
		It("should fail to start a running target", func() {
			_, err := importClient.Start(ctx, &remoteread.StartReadRequest{
				Target: target,
				Query:  query,
			})
			Expect(err).To(HaveOccurred())
		})

		It("should fail to edit a target", func() {
			_, err := importClient.EditTarget(ctx, &remoteread.TargetEditRequest{
				Meta: target.Meta,
				TargetDiff: &remoteread.TargetDiff{
					Name: "new-name",
				},
			})
			Expect(err).To(HaveOccurred())
		})

		It("should fail to delete a target", func() {
			_, err := importClient.RemoveTarget(ctx, &remoteread.TargetRemoveRequest{
				Meta: target.Meta,
			})
			Expect(err).To(HaveOccurred())
		})
	})

	//When("discovering targets", func() {
	//	It("should find all Prometheuses", func() {
	//		entries, err := importClient.DiscoverPrometheuses(ctx, &remoteread.DiscoveryRequest{
	//			ClusterIds: []string{agentId},
	//		})
	//		Expect(err).ToNot(HaveOccurred())
	//		Expect(entries).To(ContainElements(
	//			&remoteread.DiscoveryEntry{
	//				Name:             "test-prometheus",
	//				ClusterId:        "",
	//				ExternalEndpoint: "external-url.domain",
	//				InternalEndpoint: "test-prometheus.default.svc.cluster.local",
	//			},
	//			&remoteread.DiscoveryEntry{
	//				Name:             "test-prometheus",
	//				ClusterId:        "",
	//				ExternalEndpoint: "external-url.domain",
	//				InternalEndpoint: "test-prometheus.test-ns.svc.cluster.local",
	//			},
	//		))
	//	})
	//
	//	It("should limit Prometheus discovery to namespaces", func() {
	//		defaultNs := "default"
	//		testNs := testNamespace
	//
	//		entries, err := importClient.DiscoverPrometheuses(ctx, &remoteread.DiscoveryRequest{
	//			ClusterIds: []string{agentId},
	//			Namespace:  &defaultNs,
	//		})
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(entries).To(ContainElement(
	//			&remoteread.DiscoveryEntry{
	//				Name:             "test-prometheus",
	//				ClusterId:        "",
	//				ExternalEndpoint: "external-url.domain",
	//				InternalEndpoint: "test-prometheus.default.svc.cluster.local",
	//			}))
	//
	//		entries, err = importClient.DiscoverPrometheuses(ctx, &remoteread.DiscoveryRequest{
	//			ClusterIds: []string{agentId},
	//			Namespace:  &testNs,
	//		})
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(entries).To(ContainElement(&remoteread.DiscoveryEntry{
	//			Name:             "test-prometheus",
	//			ClusterId:        "",
	//			ExternalEndpoint: "external-url.domain",
	//			InternalEndpoint: "test-prometheus.test-ns.svc.cluster.local",
	//		}))
	//	})
	//})
})
