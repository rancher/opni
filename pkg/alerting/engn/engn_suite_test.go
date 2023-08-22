package engn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/engn"
	"github.com/rancher/opni/pkg/test/freeport"
	mock_engn "github.com/rancher/opni/pkg/test/mock/alerting/engn"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/samber/lo"
	"google.golang.org/grpc"
)

func TestEngn(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Engn Suite")
}

type testState struct {
	ctrl *gomock.Controller

	*mock_engn.MockSyncConstructor
	syncManager engn.SyncManager
	syncEngine  engn.SyncEngine

	addr string
}

func (ts *testState) ListenAndServeSyncEngine(ctx context.Context) {
	fp := freeport.GetFreePort()
	addr := fmt.Sprintf("127.0.0.1:%d", fp)
	serveAddr := fmt.Sprintf("tcp://%s", addr)
	ts.addr = addr

	grpcServer := grpc.NewServer()
	alertops.RegisterConfigReconcilerServer(grpcServer, ts.syncManager)
	listener, err := util.NewProtocolListener(serveAddr)
	if err != nil {
		panic(err)
	}
	errC := lo.Async(func() error {
		return grpcServer.Serve(listener)
	})
	go func() {
		select {
		case <-ctx.Done():
			grpcServer.Stop()
			listener.Close()
		case err := <-errC:
			panic(err)
		}
	}()
}
