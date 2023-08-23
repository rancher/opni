package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/waitctx"
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

type Main struct {
	Modes meta.ModeSet
}

func (m *Main) Exec() {
	gin.SetMode(gin.ReleaseMode)
	ctx, ca := context.WithCancel(waitctx.Background())
	md := meta.ReadMetadata()
	moduleBasename := path.Base(md.Module)

	mode := meta.PluginMode(os.Getenv(meta.PluginModeEnvVar))
	if !mode.IsValid() {
		panic(fmt.Sprintf("invalid plugin mode: %q", mode))
	}
	if mode == meta.ModeListModes {
		json.NewEncoder(os.Stdout).Encode(m.Modes)
		os.Exit(0)
	}

	schemeFunc, ok := m.Modes[mode]
	if !ok {
		panic("unsupported plugin mode: " + mode)
	}
	scheme := schemeFunc(ctx)

	tracing.Configure(fmt.Sprintf("plugin_%s_%s", mode, moduleBasename))

	Serve(scheme)
	ca()
	waitctx.Wait(ctx, 5*time.Second)
}
