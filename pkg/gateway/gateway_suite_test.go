package gateway_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Extension Suite")
}

type testVars struct {
	ctrl         *gomock.Controller
	client       gatewayclients.GatewayClient
	grpcEndpoint string
	interceptor  mockInterceptor
}

type mockIdentifier struct {
	id string
}

func (i *mockIdentifier) UniqueIdentifier(ctx context.Context) (string, error) {
	return i.id, nil
}

type mockInterceptor struct {
	AuthValue string
}

func (i *mockInterceptor) unaryClientInterceptor(
	ctx context.Context,
	method string,
	req interface{},
	reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", i.AuthValue)
	return invoker(ctx, method, req, reply, cc, opts...)
}

func (i *mockInterceptor) unaryServerIntercemptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		err = grpc.Errorf(codes.InvalidArgument, "no metadata in context")
		return
	}
	authHeader := md.Get("authorization")
	if len(authHeader) == 0 {
		err = grpc.Errorf(codes.InvalidArgument, "no auth header in metadata: %+v", md)
		return
	}
	if len(authHeader) > 0 && authHeader[0] == "" {
		err = grpc.Errorf(codes.InvalidArgument, "authorization header required")
		return
	}

	if authHeader[0] != i.AuthValue {
		err = status.Error(codes.Unauthenticated, "not authenticated")
		return
	}

	return handler(ctx, req)
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

		interceptor := mockInterceptor{
			AuthValue: "teststring",
		}
		tv.interceptor = interceptor
		Expect(err).NotTo(HaveOccurred())

		server := gateway.NewGRPCServer(conf, lg,
			grpc.Creds(insecure.NewCredentials()),
			grpc.ChainUnaryInterceptor(interceptor.unaryServerIntercemptor),
		)
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
