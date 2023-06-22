package update_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/noop"
	"google.golang.org/grpc"
)

func TestPatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "update suite")
}

type mockHandler struct {
	streamHandlerCalls int
	update.UpdateTypeHandler
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		UpdateTypeHandler: noop.NewNoopUpdate(),
	}
}

func (h *mockHandler) mockInterceptor() *mockInterceptor {
	return &mockInterceptor{
		mockHandler: h,
	}
}

type mockInterceptor struct {
	*mockHandler
}

func (i *mockInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		i.streamHandlerCalls++
		return handler(srv, stream)
	}
}
