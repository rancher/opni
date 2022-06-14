package gateway

import (
	"context"
	"fmt"

	"github.com/gogo/status"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	gsync "github.com/kralicky/gpkg/sync"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/types/known/emptypb"
)

type UnaryService struct {
	methodTable gsync.Map[string, *UnknownUnaryMetadata]
}

type UnknownUnaryMetadata struct {
	Conn       *grpc.ClientConn
	InputType  *desc.MessageDescriptor
	OutputType *desc.MessageDescriptor
}

func NewUnaryService() UnaryService {
	return UnaryService{
		methodTable: gsync.Map[string, *UnknownUnaryMetadata]{},
	}
}

func (u *UnaryService) RegisterUnaryPlugins(ctx waitctx.RestrictiveContext, s grpc.ServiceRegistrar, pl plugins.LoaderInterface) {
	lg := logger.New().Named("gateway.unary")
	pl.Hook(hooks.OnLoadMC(func(p types.UnaryAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
		reflectClient := grpcreflect.NewClient(ctx, rpb.NewServerReflectionClient(cc))
		sd, err := p.UnaryDescriptor(ctx, &emptypb.Empty{})
		if err != nil {
			lg.With(
				zap.Error(err),
				zap.String("plugin", md.Module),
			).Error("failed to get extension descriptor")
			return
		}
		lg.Info("got extension descriptor for service " + sd.GetName())
		waitctx.Go(ctx, func() {
			<-ctx.Done()
			reflectClient.Reset()
		})
		svcName := sd.GetName()
		svcDesc, err := reflectClient.ResolveService(svcName)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to resolve extension service")
			return
		}
		newSd := grpc.ServiceDesc{
			ServiceName: sd.GetName(),
			HandlerType: (*interface{})(nil),
			Streams:     []grpc.StreamDesc{},
			Methods:     []grpc.MethodDesc{},
		}
		lg.With(
			"name", svcName,
		).Info("loading service")
		for _, mtd := range svcDesc.GetMethods() {
			fullName := fmt.Sprintf("/%s/%s", svcName, mtd.GetName())
			lg.With(
				"name", fullName,
			).Info("loading method")

			u.methodTable.Store(fullName, &UnknownUnaryMetadata{
				Conn:       cc,
				InputType:  mtd.GetInputType(),
				OutputType: mtd.GetOutputType(),
			})

			newSd.Methods = append(newSd.Methods, grpc.MethodDesc{
				MethodName: mtd.GetName(),
				Handler:    u.Handler,
			})
		}
		s.RegisterService(&newSd, u)
	}))
}

func (u *UnaryService) Handler(
	srv interface{},
	ctx context.Context,
	dec func(interface{}) error,
	interceptor grpc.UnaryServerInterceptor,
) (interface{}, error) {
	stream := grpc.ServerTransportStreamFromContext(ctx)
	fullMethodName := stream.Method()
	if conn, ok := u.methodTable.Load(fullMethodName); ok {
		md, _ := metadata.FromIncomingContext(ctx)
		outgoingCtx := metadata.NewOutgoingContext(ctx, md.Copy())
		request := dynamic.NewMessage(conn.InputType)
		if err := dec(request); err != nil {
			return nil, err
		}
		reply := dynamic.NewMessage(conn.OutputType)
		if interceptor == nil {
			err := conn.Conn.Invoke(outgoingCtx, fullMethodName, request, reply)
			if err != nil {
				return nil, err
			}
			return reply, err
		}
		info := &grpc.UnaryServerInfo{
			Server:     srv,
			FullMethod: fullMethodName,
		}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			reply := dynamic.NewMessage(conn.OutputType)
			err := conn.Conn.Invoke(outgoingCtx, fullMethodName, req, reply)
			if err != nil {
				return nil, err
			}
			return reply, err
		}
		return interceptor(ctx, request, info, handler)
	}
	return nil, status.Errorf(codes.Internal, "method does not exist")
}
