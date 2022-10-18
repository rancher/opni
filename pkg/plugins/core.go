package plugins

import (
	"context"
	"errors"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/plugins/meta"
	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

var ErrNotImplemented = errors.New("not implemented")

var (
	GatewayScheme = meta.NewScheme(meta.WithMode(meta.ModeGateway))
	AgentScheme   = meta.NewScheme(meta.WithMode(meta.ModeAgent))
)

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  plugin.CoreProtocolVersion,
	MagicCookieKey:   "OPNI_MAGIC_COOKIE",
	MagicCookieValue: "opni",
}

func CheckAvailability(ctx context.Context, cc *grpc.ClientConn, id string) error {
	ref := rpb.NewServerReflectionClient(cc)
	stream, err := ref.ServerReflectionInfo(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_ListServices{},
	}); err != nil {
		return err
	}
	response, err := stream.Recv()
	if err != nil {
		return err
	}
	for _, svc := range response.GetListServicesResponse().GetService() {
		if svc.Name == id {
			return nil
		}
	}
	return ErrNotImplemented
}
