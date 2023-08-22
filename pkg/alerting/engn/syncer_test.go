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
	mock_engn "github.com/rancher/opni/pkg/test/mock/alerting/engn"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	syncId     = "test-sync-id"
	syncData   = []byte("test-sync-data")
	configHash = uuid.New().String()
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
	stream     alertops.ConfigReconciler_SyncConfigClient
}

func connectClientFixture(ctx context.Context, ts *testState) clientWrapper {
	By("connecting a client to the syncer engine")
	cc, err := setupConn(ts.addr)
	Expect(err).NotTo(HaveOccurred())
	client := alertops.NewConfigReconcilerClient(cc)
	stream, err := client.SyncConfig(context.Background())
	Expect(err).NotTo(HaveOccurred())

	recvErr := make(chan error)
	receivedSyncs := make(chan *alertops.SyncRequest)
	assignedId := ""
	syncId := ""
	go func() {
		for {
			select {
			case <-ctx.Done():
				recvErr <- ctx.Err()
				return
			default:
				syncReq, err := stream.Recv()
				if err != nil {
					fmt.Println(err)
					recvErr <- err
					return
				}
				fmt.Println(syncReq.LifecycleId)
				assignedId = syncReq.LifecycleId
				syncId = syncReq.SyncId
				receivedSyncs <- syncReq
			}
		}
	}()

	By("verifying the downstream syncer received the request without errors")
	select {
	case <-time.After(time.Second):
		Fail("timed out waiting for sync request")
	case err := <-recvErr:
		Expect(err).NotTo(HaveOccurred())
	case syncReq := <-receivedSyncs:
		Expect(syncReq).NotTo(BeNil())
	}
	return clientWrapper{
		cc:         cc,
		assignedId: assignedId,
		syncId:     syncId,
		msgQ:       receivedSyncs,
		stream:     stream,
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
			defer cw.cc.Close()
			Expect(cw.assignedId).NotTo(BeEmpty())
			Expect(cw.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(1))

			By("verifying the syncer engine has not registered the downstream's syncer remote info until a response has been received")
			Expect(len(ts.syncManager.Info())).To(Equal(0))

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

			info := ts.syncManager.Info()
			Expect(info).To(HaveKey(cw.assignedId))
			Expect(info[cw.assignedId].WhoAmI).To(Equal("test"))
			Expect(info[cw.assignedId].LastSyncState).To(Equal(alertops.SyncState_Synced))
			By("verifying when the client disconnects the downstream syncer is no longer registered")
			err = cw.stream.CloseSend()
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() int {
				return len(ts.syncManager.Info())
			}).Should(Equal(0))

		})

		It("should be able to push sync requests to downstream syncers", func() {
			By("initially verifying no syncers are connected")
			Expect(len(ts.syncManager.Info())).To(Equal(0))
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()

			By("verifying the clients were registered correctly")
			cw := connectClientFixture(ctxCa, ts)
			defer cw.cc.Close()
			Expect(cw.assignedId).NotTo(BeEmpty())
			Expect(cw.syncId).NotTo(BeEmpty())
			Expect(len(ts.syncManager.Targets())).To(Equal(1))
			Expect(ts.syncManager.Targets()).To(ConsistOf([]string{cw.assignedId}))
			cw2 := connectClientFixture(ctxCa, ts)
			defer cw2.cc.Close()
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
			select {
			case <-time.After(time.Millisecond * 100):
				Fail("timed out waiting for sync request")
			case syncReq := <-cw.msgQ:
				Expect(syncReq).NotTo(BeNil())
				Expect(syncReq.SyncId).To(Equal("test-sync-id"))
				Expect(syncReq.Items).To(HaveLen(1))
				Expect(syncReq.Items[0].Key).To(Equal("test-key"))
				Expect(syncReq.Items[0].Config).To(Equal([]byte("test-data")))
			}

			By("verifying the second connected client received the broadcasted sync payload")
			select {
			case <-time.After(time.Second):
				Fail("timed out waiting for sync request")
			case syncReq := <-cw2.msgQ:
				Expect(syncReq).NotTo(BeNil())
				Expect(syncReq.SyncId).To(Equal("test-sync-id"))
				Expect(syncReq.Items).To(HaveLen(1))
				Expect(syncReq.Items[0].Key).To(Equal("test-key"))
				Expect(syncReq.Items[0].Config).To(Equal([]byte("test-data")))
			}

			By("verifying it can push targeted syncs")
			ts.syncManager.PushTarget(cw.assignedId, engn.SyncPayload{
				SyncId:    "test-cw",
				ConfigKey: "test-key",
				Data:      []byte("test-cw"),
			})
			By("verifying the targeted syncer received the sync payload")

			// TODO : this is super flaky
			select {
			case <-time.After(10 * time.Second):
				Fail("timed out waiting for sync request")
			case syncReq := <-cw.msgQ:
				Expect(syncReq).NotTo(BeNil())
				Expect(syncReq.SyncId).To(Equal("test-cw"))
				Expect(syncReq.Items).To(HaveLen(1))
				Expect(syncReq.Items[0].Key).To(Equal("test-key"))
				Expect(syncReq.Items[0].Config).To(Equal([]byte("test-cw")))
			}

			By("verifying the non-targeted syncer did not receive the sync payload")
			Consistently(func() int {
				return len(cw2.msgQ)
			}).Should(Equal(0))
		})
	})

	When("we use the syncer engine", func() {
		It("should push syncs when the configuration has changed in the engine", func() {
			ctxCa, ca := context.WithCancel(context.Background())
			defer ca()
			cw := connectClientFixture(context.Background(), ts)
			defer cw.cc.Close()

			Expect(ts.syncManager.Targets()).To(HaveLen(1))

			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err := ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			select {
			case <-time.After(3 * time.Second):
				Fail("timed out waiting for sync request")
			case syncMsg := <-cw.msgQ:
				Expect(syncMsg).NotTo(BeNil())
				Expect(syncMsg.SyncId).To(Equal(syncId))
				Expect(syncMsg.Items).To(HaveLen(1))
				Expect(syncMsg.Items[0].Key).To(Equal("key"))
				Expect(syncMsg.Items[0].Config).To(Equal([]byte(configHash)))

			}
			configHash = uuid.New().String()
			syncId = configHash
			syncData = []byte(configHash)
			err = ts.syncEngine.Sync(ctxCa, sync.SyncInfo{
				ShouldSync: true,
			})
			Expect(err).NotTo(HaveOccurred())
			select {
			case <-time.After(time.Millisecond * 100):
				Fail("timed out waiting for sync request")
			case syncMsg := <-cw.msgQ:
				Expect(syncMsg).NotTo(BeNil())
				Expect(syncMsg.SyncId).To(Equal(syncId))
				Expect(syncMsg.Items).To(HaveLen(1))
				Expect(syncMsg.Items[0].Key).To(Equal("key"))
				Expect(syncMsg.Items[0].Config).To(Equal([]byte(configHash)))
			}
		})

		It("should push syncs when the a syncer has an out-of-date syncId", func() {
		})

		It("should push syncs when the a syncer did not successfully receive the last sync", func() {

		})
	})
})
