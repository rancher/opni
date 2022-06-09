package gateway_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	gatewayclients "github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/waitctx"
)

func TestManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Suite")
}

type testVars struct {
	ctrl         *gomock.Controller
	client       gatewayclients.GatewayGRPCClient
	grpcEndpoint string
}

type mockIdentifier struct {
	id string
}

func (i *mockIdentifier) UniqueIdentifier(ctx context.Context) (string, error) {
	return i.id, nil
}

func setupGatewayGRPCServer(vars **testVars, pl plugins.LoaderInterface) func() {
	return func() {
		tv := &testVars{}
		if *vars != nil && (*vars).ctrl != nil {
			tv.ctrl = (*vars).ctrl
		} else {
			tv.ctrl = gomock.NewController(GinkgoT())
		}

		lg := logger.New().Named("gateway_test")

		ports, err := freeport.GetFreePorts(1)
		Expect(err).NotTo(HaveOccurred())

		ctx, ca := context.WithCancel(waitctx.Background())

		conf := &v1beta1.GatewayConfigSpec{
			GRPCListenAddress: fmt.Sprintf("127.0.0.1:%d", ports[0]),
		}

		Expect(err).NotTo(HaveOccurred())

		server := gateway.NewGRPCServer(conf, lg)
		unarySvc := gateway.NewUnaryService()
		unarySvc.RegisterUnaryPlugins(ctx, server, pl)
		tv.grpcEndpoint = fmt.Sprintf("127.0.0.1:%d", ports[0])

		pl.Hook(hooks.OnLoadingCompleted(func(int) {
			defer GinkgoRecover()
			if err := server.ListenAndServe(ctx); err != nil {
				test.Log.Error(err)
			}
		}))
		Expect(err).NotTo(HaveOccurred())

		*vars = tv

		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx, 5*time.Second)
		})
	}
}
