package auth_test

import (
	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/test"
	"google.golang.org/grpc"
)

type testHttp struct{}

func (*testHttp) Handle(ctx *fiber.Ctx) error {
	return nil
}

type testUnaryGrpc struct{}

func (*testUnaryGrpc) UnaryServerInterceptor() grpc.UnaryClientInterceptor {
	return nil
}

type testStreamGrpc struct{}

func (*testStreamGrpc) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return nil
}

var _ = Describe("Middleware", Label("unit"), func() {
	It("should detect supported protocols", func() {
		Expect(auth.SupportedProtocols(&testHttp{})).To(Equal(auth.ProtocolHTTP))
		Expect(auth.SupportedProtocols(&testUnaryGrpc{})).To(Equal(auth.ProtocolUnaryGRPC))
		Expect(auth.SupportedProtocols(&testStreamGrpc{})).To(Equal(auth.ProtocolStreamGRPC))
		Expect(auth.SupportedProtocols(&struct {
			*testHttp
			*testUnaryGrpc
		}{})).To(Equal(auth.ProtocolHTTP | auth.ProtocolUnaryGRPC))
		Expect(auth.SupportedProtocols(&struct {
			*testHttp
			*testStreamGrpc
		}{})).To(Equal(auth.ProtocolHTTP | auth.ProtocolStreamGRPC))
		Expect(auth.SupportedProtocols(&struct {
			*testUnaryGrpc
			*testStreamGrpc
		}{})).To(Equal(auth.ProtocolUnaryGRPC | auth.ProtocolStreamGRPC))
		Expect(auth.SupportedProtocols(&struct {
			*testHttp
			*testUnaryGrpc
			*testStreamGrpc
		}{})).To(Equal(auth.ProtocolHTTP | auth.ProtocolUnaryGRPC | auth.ProtocolStreamGRPC))
	})
})
