package plugins

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

var ErrNotImplemented = errors.New("not implemented")

var ClientScheme = meta.NewScheme()

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  plugin.CoreProtocolVersion,
	MagicCookieKey:   "OPNI_MONITORING_MAGIC_COOKIE",
	MagicCookieValue: "opni-monitoring",
}

func init() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		plugin.CleanupClients()
		os.Exit(0)
	}()
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
