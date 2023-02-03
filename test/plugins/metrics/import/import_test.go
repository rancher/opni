package _import

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"time"
)

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

var _ = Describe("Remote Read Import", Ordered, Label(test.Integration, test.Slow), func() {
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
		env = &test.Environment{
			EnvironmentOptions: test.EnvironmentOptions{},
			TestBin:            "../../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		managementClient := env.NewManagementClient()
		token, err := managementClient.CreateBootstrapToken(ctx, &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).ToNot(HaveOccurred())

		certInfo, err := managementClient.CertsInfo(ctx, &emptypb.Empty{})
		Expect(err).ToNot(HaveOccurred())

		env.StartAgent(agentId, token, []string{certInfo.Chain[len(certInfo.Chain)-1].Fingerprint})

		importClient = remoteread.NewRemoteReadGatewayClient(env.ManagementClientConn())

		port, err := freeport.GetFreePort()
		Expect(err).ToNot(HaveOccurred())

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
			Expect(status.State).To(Equal(remoteread.TargetStatus_Running))
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
})
