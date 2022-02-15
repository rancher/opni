package example

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni-monitoring/pkg/management"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ExamplePlugin struct {
	UnimplementedExampleAPIExtensionServer
	Logger hclog.Logger
}

func (s *ExamplePlugin) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}

func (s *ExamplePlugin) UseManagementAPI(api management.ManagementClient) {
	lg := s.Logger
	lg.Info("querying management API...")
	var list *management.APIExtensionInfoList
	for {
		var err error
		list, err = api.APIExtensions(context.Background(), &emptypb.Empty{})
		if err != nil {
			log.Fatal(err)
		}
		if len(list.Items) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	for _, ext := range list.Items {
		lg.Info("found API extension service", "name", ext.ServiceDesc.GetName())
	}
}
