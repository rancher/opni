package extensions_test

//
//import (
//	"context"
//	"fmt"
//	"time"
//
//	. "github.com/onsi/ginkgo/v2"
//	. "github.com/onsi/gomega"
//	"github.com/rancher/opni/pkg/alerting/drivers/backend"
//	"github.com/rancher/opni/pkg/alerting/extensions"
//	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
//	"github.com/rancher/opni/pkg/test"
//	"google.golang.org/protobuf/types/known/emptypb"
//	"google.golang.org/protobuf/types/known/timestamppb"
//)
//
//func BuildSyncerServerTestSuite(
//	name string,
//	serverConstructor func(ctx context.Context) alertingv1.SyncerServer,
//) bool {
//	return Describe(name, Ordered, Label(test.Unit, test.Slow), func() {
//		var server alertingv1.SyncerServer
//		var initialTimestamp *timestamppb.Timestamp
//		var ctx context.Context
//		BeforeAll(func() {
//			Expect(env).NotTo(BeNil())
//			Expect(embeddedJetstream).NotTo(BeNil())
//			Expect(configFilePath).NotTo(Equal(""))
//			Expect(tmpConfigDir).NotTo(Equal(""))
//			Expect(opniPort).NotTo(Equal(0))
//			Expect(alertmanagerPort).NotTo(Equal(0))
//			ctx = env.Context()
//			Expect(ctx).NotTo(BeNil())
//
//			By("verifying an active opni alertmanager instance is running")
//			url := fmt.Sprintf("http://localhost:%d", alertmanagerPort)
//			apiNode := backend.NewAlertManagerReadyClient(
//				env.Context(),
//				url,
//				backend.WithDefaultRetrier(),
//				backend.WithExpectClosure(backend.NewExpectStatusOk()),
//			)
//			err := apiNode.DoRequest()
//			Expect(err).NotTo(HaveOccurred())
//			err = storageClient.Purge(env.Context())
//			Expect(err).NotTo(HaveOccurred(), "failed to purge data from the storage node")
//
//			server = serverConstructor(ctx)
//			Expect(server).NotTo(BeNil())
//
//		})
//		When("we start the embedded server", func() {
//			//TODO: FIXME
//			It("should perform a successful initial sync", func(ctx context.Context) {
//				By("verifying that the server is eventually ready and healthy")
//				Eventually(func() error {
//					if _, err := server.Ready(ctx, &emptypb.Empty{}); err != nil {
//						return fmt.Errorf("server not ready : %s", err)
//					}
//
//					if _, err := server.Healthy(ctx, &emptypb.Empty{}); err != nil {
//						return fmt.Errorf("server not healthy : %s", err)
//					}
//					return nil
//				}).Should(Succeed())
//
//				By("verfying the server can return its status")
//				statusMsg, err := server.Status(ctx, &emptypb.Empty{})
//				Expect(err).NotTo(HaveOccurred())
//				Expect(statusMsg).NotTo(BeNil())
//				Expect(statusMsg.LastSynced).NotTo(BeNil())
//				Expect(initialTimestamp.AsTime().Before(statusMsg.LastSynced.AsTime())).To(BeTrue())
//			}, SpecTimeout(time.Minute*2))
//
//			//It("should be able to perform a successful sync", func() {
//			//	storageClient := storage.NewClientSet(storage.NewStorageAPIs(embeddedJetstream, 24*time.Hour))
//			//	for i := 0; i < 10; i++ {
//			//		endpId := uuid.New().String()
//			//		err := storageClient.Endpoints.Put(ctx, endpId, &alertingv1.AlertEndpoint{
//			//			Name:        "test-endpoint",
//			//			Description: "test-endpoint",
//			//			Endpoint: &alertingv1.AlertEndpoint_Slack{
//			//				Slack: &alertingv1.SlackEndpoint{
//			//					WebhookUrl: "https://slack.com",
//			//					Channel:    "#channel",
//			//				},
//			//			},
//			//			Id:          endpId,
//			//			LastUpdated: timestamppb.Now(),
//			//		})
//			//		Expect(err).NotTo(HaveOccurred())
//			//		condId := uuid.New().String()
//			//		err = storageClient.Conditions.Put(ctx, condId, &alertingv1.AlertCondition{
//			//			Id:          condId,
//			//			LastUpdated: timestamppb.Now(),
//			//			AttachedEndpoints: &alertingv1.AttachedEndpoints{
//			//				Items: []*alertingv1.AttachedEndpoint{
//			//					{
//			//						EndpointId: endpId,
//			//					},
//			//				},
//			//				Details: &alertingv1.EndpointImplementation{
//			//					Title: "test",
//			//					Body:  "test",
//			//				},
//			//			},
//			//		})
//			//		Expect(err).NotTo(HaveOccurred())
//			//	}
//
//				//_, err := server.Sync(ctx, &emptypb.Empty{})
//				//Expect(err).NotTo(HaveOccurred())
//				//statusMsg, err := server.Status(ctx, &emptypb.Empty{})
//				//Expect(err).NotTo(HaveOccurred())
//				//Expect(statusMsg).NotTo(BeNil())
//				//Expect(statusMsg.LastSynced).NotTo(BeNil())
//				//Expect(initialTimestamp.AsTime().Before(statusMsg.LastSynced.AsTime())).To(BeTrue())
//				//
//				////TODO: check the configurations have actually been loaded into alertmanager
//			})
//	})
//}
//func init() {
//	var _ = BuildSyncerServerTestSuite(
//		"Alertmanager sidecar Syncer",
//		func(ctx context.Context) alertingv1.SyncerServer {
//			return extensions.NewAlertingSyncerV1(
//				ctx,
//				&alertingv1.SyncerConfig{
//					AlertmanagerConfigPath: configFilePath,
//					AlertmanagerAddress:    fmt.Sprintf("http://localhost:%d", alertmanagerPort),
//					HookListenAddress:      "https://localhost:3001/default/opni.hook",
//				},
//				extensions.WithJetStream(embeddedJetstream),
//			)
//		})
//}
