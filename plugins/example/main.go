package main

import (
	context "context"

	"github.com/kralicky/opni-monitoring/pkg/plugins"
	"github.com/kralicky/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/kralicky/opni-monitoring/pkg/plugins/meta"
)

func main() {
	scheme := meta.NewScheme()
	p := apiextensions.NewPlugin(&ExampleAPIExtension_ServiceDesc, &echoServer{})
	scheme.Add(apiextensions.ManagementAPIExtensionPluginID, p)

	plugins.Serve(scheme)
}

type echoServer struct {
	UnimplementedExampleAPIExtensionServer
}

func (s *echoServer) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{
		Message: req.Message,
	}, nil
}
