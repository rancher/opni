package engn_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/engn"
	"github.com/rancher/opni/pkg/alerting/server/sync"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	mock_engn "github.com/rancher/opni/pkg/test/mock/alerting/engn"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	syncId     = "test-sync-id"
	syncData   = []byte("test-sync-data")
	configHash = uuid.New().String()
	// lg         = logger.NewPluginLogger().Named("test")
)

func setupConn(remoteAddr string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return conn, err
	}
	return conn, nil
}

type clientWrapper struct {
	cc         *grpc.ClientConn
	assignedId string
	syncId     string
	msgQ       chan *alertops.SyncRequest
	recvErr    chan error
	stream     alertops.ConfigReconciler_SyncConfigClient
}

func connectClientFixture(ctx context.Context, ts *testState) clientWrapper {
	By("connecting a client to the syncer engine")
	cc, err := setupConn(ts.addr)
	Expect(err).NotTo(HaveOccurred())
	client := alertops.NewConfigReconcilerClient(cc)
	stream, err := client.SyncConfig(context.Background())
	Expect(err).NotTo(HaveOccurred())

	recvErr := make(chan error, 256)
	receivedSyncs := make(chan *alertops.SyncRequest, 256)
	assignedId := ""
	syncId := ""
	go func() {
		for {
			syncReq, err := stream.Recv()
			// lg.Debug("received something from stream")
			if err != nil {
				// lg.With("client", "recv").Error(err)
				fmt.Println(err)
				recvErr <- err
				return
			}
			// lg.With("lifecycleId", syncReq.LifecycleId, "syncId", syncReq.SyncId).Debug("received sync request")
			assignedId = syncReq.LifecycleId
			syncId = syncReq.SyncId
			// lg.With("lifecycleId", syncReq.LifecycleId, "syncId", syncReq.SyncId).Debug("sending through chan")
			receivedSyncs <- syncReq
			// lg.With("lifecycleId", syncReq.LifecycleId, "syncId", syncReq.SyncId).Debug("sent through chan")

		}
	}()

	By("verifying the downstream syncer received the request without errors")
	Eventually(receivedSyncs).Should(Receive())
	Consistently(recvErr).ShouldNot(Receive())
	return clientWrapper{
		cc:         cc,
		assignedId: assignedId,
		syncId:     syncId,
		msgQ:       receivedSyncs,
		stream:     stream,
		recvErr:    recvErr,
	}
}

var _ = Describe("Alerting gateway syncer engine", func() {
	var ts *testState
	JustBeforeEach(func() {
		ctxCa, ca := context.WithCancel(context.Background())
		ctrl := gomock.NewController(GinkgoT())
		mockSyncConstructor := mock_engn.NewMockSyncConstructor(ctrl)
		mockSyncConstructor.EXPECT().
			Construct().
			DoAndReturn(func() (engn.SyncPayload, error) {
				id := syncId
				return engn.SyncPayload{
					SyncId:    id,
					ConfigKey: "key",
					Data:      syncData,
				}, nil
			}).
			AnyTimes()

		mockSyncConstructor.EXPECT().
			GetHash().
			DoAndReturn(func() (string, error) {
				return configHash, nil
			}).
			AnyTimes()

		syncM := engn.NewSyncManager(ctxCa, mockSyncConstructor)

		syncE := engn.NewSyncEngine(ctxCa, mockSyncConstructor, &drivers.NoopClusterDriver{}, syncM)

		ts = &testState{
			ctrl:                ctrl,
			MockSyncConstructor: mockSyncConstructor,
			syncManager:         syncM,
			syncEngine:          syncE,
		}

		ts.ListenAndServeSyncEngine(ctxCa)

		DeferCleanup(ca)
	})

	When("we use the sync manager", func() {
		It("should register/unregister downstream syncers", func() {
			By("initially verifying no syncers are connected")
			Expect(len(ts.syncManager.Info())).To(Equal(0))
			Expect(len(ts.syncManager.Targets())).To(Equal(0))

			By("verifying the client was registered correctly")
			cw := connectClientFixture(context.Background(), ts)
			Expect(cw.assignedId).NotTo(BeEmpty())
			Expect(cw.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(1))

			By("verifying the syncer engine has not registered the downstream's syncer remote info until a response has been received")
			Eventually(len(ts.syncManager.Info())).Should(Equal(1))
			Expect(ts.syncManager.Info()).To(HaveKey(cw.assignedId))
			Expect(ts.syncManager.Info()[cw.assignedId].LastSyncId).To(Equal(""))

			By("verifying the syncer engine registers a downstream syncer when it receives a response")
			err := cw.stream.Send(&alertops.ConnectInfo{
				LifecycleUuid: cw.assignedId,
				Whoami:        "test",
				State:         alertops.SyncState_Synced,
				SyncId:        cw.syncId,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				return len(ts.syncManager.Info())
			}).Should(Equal(1))

			Eventually(ts.syncManager.Info()).Should(HaveKey(cw.assignedId))
			info := ts.syncManager.Info()[cw.assignedId]
			fmt.Println(info)
			fmt.Println(cw.syncId)
			Eventually(ts.syncManager.Info()[cw.assignedId].LastSyncId).Should(Equal(cw.syncId))
			Eventually(ts.syncManager.Info()[cw.assignedId].LastSyncState).Should(Equal(alertops.SyncState_Synced))
			By("verifying when the client disconnects the downstream syncer is no longer registered")
			err = cw.stream.CloseSend()
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				return len(ts.syncManager.Info())
			}).Should(Equal(0))

		})

		It("should push non-duplicate syncs downstream", func() {
			By("initially verifying no syncers are connected")
			Expect(len(ts.syncManager.Info())).To(Equal(0))
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()

			By("verifying the clients were registered correctly")
			cw := connectClientFixture(ctxCa, ts)
			Expect(cw.assignedId).NotTo(BeEmpty())
			Expect(cw.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(1))
			Expect(ts.syncManager.Targets()).To(ConsistOf([]string{cw.assignedId}))

			id := uuid.New().String()
			ts.syncManager.Push(engn.SyncPayload{
				SyncId:    id,
				ConfigKey: "test-key",
				Data:      []byte("test-data"),
			})

			Eventually(cw.msgQ).Should(Receive(testutil.ProtoEqual(&alertops.SyncRequest{
				LifecycleId: cw.assignedId,
				SyncId:      id,
				Items: []*alertingv1.PutConfigRequest{
					{
						Key:    "test-key",
						Config: []byte("test-data"),
					},
				},
			})))
			Consistently(cw.recvErr).ShouldNot(Receive())

			err := cw.stream.Send(&alertops.ConnectInfo{
				LifecycleUuid: cw.assignedId,
				Whoami:        "test",
				State:         alertops.SyncState_Synced,
				SyncId:        cw.syncId,
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				info := ts.syncManager.Info()[cw.assignedId]
				if info.LastSyncId != cw.syncId {
					return fmt.Errorf("expected syncId '%s', got '%s'", cw.syncId, info.LastSyncId)
				}
				if info.LastSyncState != alertops.SyncState_Synced {
					return fmt.Errorf("expected syncState '%s', got '%s'", alertops.SyncState_Synced, info.LastSyncState)
				}
				return nil
			}, time.Second, time.Millisecond*500).Should(Succeed())

			ts.syncManager.Push(engn.SyncPayload{
				SyncId:    id,
				ConfigKey: "test-key",
				Data:      []byte("test-data"),
			})
			By("verifying the downstream syncer does not receive duplicate syncs")
			Consistently(cw.msgQ).ShouldNot(Receive())

			id2 := uuid.New().String()
			ts.syncManager.Push(engn.SyncPayload{
				SyncId:    id2,
				ConfigKey: "test-key",
				Data:      []byte("test-data2"),
			})

			Eventually(cw.msgQ).Should(Receive(testutil.ProtoEqual(&alertops.SyncRequest{
				LifecycleId: cw.assignedId,
				SyncId:      id2,
				Items: []*alertingv1.PutConfigRequest{
					{
						Key:    "test-key",
						Config: []byte("test-data2"),
					},
				},
			})))

			err = cw.stream.Send(&alertops.ConnectInfo{
				LifecycleUuid: cw.assignedId,
				Whoami:        "test",
				State:         alertops.SyncState_Synced,
				SyncId:        cw.syncId,
			})
			Expect(err).NotTo(HaveOccurred())

		})

		It("should not arbitrarily block on a bunch of sync requests", func() {
			By("initially verifying no syncers are connected")
			Expect(len(ts.syncManager.Info())).To(Equal(0))
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()

			By("verifying the clients were registered correctly")
			cw := connectClientFixture(ctxCa, ts)
			Expect(cw.assignedId).NotTo(BeEmpty())
			Expect(cw.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(1))
			Expect(ts.syncManager.Targets()).To(ConsistOf([]string{cw.assignedId}))

			for i := 0; i < 25; i++ {
				ts.syncManager.Push(engn.SyncPayload{
					SyncId:    "test-sync-id",
					ConfigKey: "test-key",
					Data:      []byte("test-data"),
				})
			}
			Eventually(len(cw.msgQ)).ShouldNot(Equal(0))
		})

		It("should be able to push sync requests to downstream syncers", func() {
			By("initially verifying no syncers are connected")
			Expect(len(ts.syncManager.Info())).To(Equal(0))
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()

			By("verifying the clients were registered correctly")
			cw := connectClientFixture(ctxCa, ts)
			Expect(cw.assignedId).NotTo(BeEmpty())
			Expect(cw.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(1))
			Expect(ts.syncManager.Targets()).To(ConsistOf([]string{cw.assignedId}))
			cw2 := connectClientFixture(ctxCa, ts)
			Expect(cw2.assignedId).NotTo(BeEmpty())
			Expect(cw2.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(2))
			Expect(ts.syncManager.Targets()).To(ConsistOf([]string{cw.assignedId, cw2.assignedId}))

			By("verifying it can broadcast to all connected syncers")
			ts.syncManager.Push(engn.SyncPayload{
				SyncId:    "test-sync-id",
				ConfigKey: "test-key",
				Data:      []byte("test-data"),
			})

			By("verifying the first connect client receive the broadcasted sync payload")
			Eventually(cw.msgQ).Should(Receive())
			Consistently(cw.recvErr).ShouldNot(Receive())

			By("verifying the second connected client received the broadcasted sync payload")
			Eventually(cw2.msgQ).Should(Receive())
			Consistently(cw2.recvErr).ShouldNot(Receive())

			By("verifying it can push targeted syncs")
			ts.syncManager.PushTarget(cw.assignedId, engn.SyncPayload{
				SyncId:    "test-cw",
				ConfigKey: "test-key",
				Data:      []byte("test-cw"),
			})
			By("verifying the targeted syncer received the sync payload")

			// TODO : why do is this randomly blocking
			Eventually(cw.msgQ).Should(Receive())
			Consistently(cw.recvErr).ShouldNot(Receive())
			Consistently(cw2.msgQ).ShouldNot(Receive())
			Consistently(cw2.recvErr).ShouldNot(Receive())
		})
	})

	When("we use the syncer engine", func() {
		It("should push syncs when the configuration has changed in the engine", func() {
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()
			cw := connectClientFixture(context.Background(), ts)

			Expect(ts.syncManager.Targets()).To(HaveLen(1))

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err := ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(cw.msgQ).Should(Receive())

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err = ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(cw.msgQ).Should(Receive())
		})

		It("should push syncs when the syncer has an out-of-date syncId", func() {
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()
			cw := connectClientFixture(context.Background(), ts)

			Expect(ts.syncManager.Targets()).To(HaveLen(1))

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err := ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(cw.msgQ).Should(Receive())

			err = cw.stream.Send(&alertops.ConnectInfo{
				LifecycleUuid: cw.assignedId,
				Whoami:        "test",
				State:         alertops.SyncState_Synced,
				SyncId:        "out-of-date",
			})
			Expect(err).NotTo(HaveOccurred())

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err = ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(cw.msgQ).Should(Receive())
		})

		It("should push syncs when the syncer did not successfully receive the last sync", func() {
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()
			cw := connectClientFixture(context.Background(), ts)

			Expect(ts.syncManager.Targets()).To(HaveLen(1))

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err := ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(cw.msgQ).Should(Receive())

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err = ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(cw.msgQ).Should(Receive())
		})
	})
})
