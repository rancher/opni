package main

import (
	context "context"
	"log"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/plugins"
	"github.com/kralicky/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/kralicky/opni-monitoring/pkg/plugins/apis/system"
	"github.com/kralicky/opni-monitoring/pkg/plugins/meta"
	"google.golang.org/protobuf/types/known/emptypb"
)

func main() {
	scheme := meta.NewScheme()
	example := &examplePlugin{}
	scheme.Add(apiextensions.ManagementAPIExtensionPluginID,
		apiextensions.NewPlugin(&ExampleAPIExtension_ServiceDesc, example))
	scheme.Add(system.SystemPluginID, system.NewPlugin(example))

	plugins.Serve(scheme)
}

type examplePlugin struct {
	UnimplementedExampleAPIExtensionServer
}

func (s *examplePlugin) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}

func (s *examplePlugin) UseManagementAPI(api management.ManagementClient) {
	log.Println("[example] querying management API...")
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
		log.Println("[example] found api extension service: " + ext.ServiceDesc.GetName())
	}
}
