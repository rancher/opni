package configv1

import (
	context "context"
	"strings"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/reactive"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	storage "github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protopath "google.golang.org/protobuf/reflect/protopath"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

type GatewayConfigManagerOptions struct {
	controllerOptions []reactive.ControllerOption
}

type GatewayConfigManagerOption func(*GatewayConfigManagerOptions)

func (o *GatewayConfigManagerOptions) apply(opts ...GatewayConfigManagerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithControllerOptions(controllerOptions ...reactive.ControllerOption) GatewayConfigManagerOption {
	return func(o *GatewayConfigManagerOptions) {
		o.controllerOptions = append(o.controllerOptions, controllerOptions...)
	}
}

type GatewayConfigManager struct {
	*driverutil.BaseConfigServer[
		*driverutil.GetRequest,
		*SetRequest,
		*ResetRequest,
		*driverutil.ConfigurationHistoryRequest,
		*HistoryResponse,
		*GatewayConfigSpec,
	]

	*reactive.Controller[*GatewayConfigSpec]
}

func NewGatewayConfigManager(
	defaultStore, activeStore storage.ValueStoreT[*GatewayConfigSpec],
	loadDefaultsFunc driverutil.DefaultLoaderFunc[*GatewayConfigSpec],
	opts ...GatewayConfigManagerOption,
) *GatewayConfigManager {
	options := GatewayConfigManagerOptions{}
	options.apply(opts...)

	g := &GatewayConfigManager{}
	g.BaseConfigServer = g.Build(defaultStore, activeStore, loadDefaultsFunc)
	g.Controller = reactive.NewController(g.Tracker(), options.controllerOptions...)
	return g
}

func (s *GatewayConfigManager) DryRun(ctx context.Context, req *DryRunRequest) (*DryRunResponse, error) {
	res, err := s.Tracker().DryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: res.ValidationErrors.ToProto(),
	}, nil
}

func (s *GatewayConfigManager) WatchReactive(in *ReactiveWatchRequest, stream GatewayConfig_WatchReactiveServer) error {
	rvs := make([]reactive.Value, 0, len(in.Paths))
	for _, pathstr := range in.Paths {
		path := protopath.Path(ProtoPath())
		for _, p := range strings.Split(pathstr, ".") {
			prev := path.Index(-1)
			switch prev.Kind() {
			case protopath.RootStep:
				field := prev.MessageDescriptor().Fields().ByName(protoreflect.Name(p))
				if field == nil {
					return status.Errorf(codes.InvalidArgument, "invalid path %q in field mask: no such field %q in message %s",
						pathstr, p, prev.MessageDescriptor().FullName())
				}
				path = append(path, protopath.FieldAccess(field))
			case protopath.FieldAccessStep:
				msg := prev.FieldDescriptor().Message()
				if msg == nil {
					return status.Errorf(codes.InvalidArgument, "invalid path %q in field mask: field %q in message %s is not a message",
						pathstr, prev.FieldDescriptor().Name(), prev.MessageDescriptor().FullName())
				}
				field := msg.Fields().ByName(protoreflect.Name(p))
				if field == nil {
					return status.Errorf(codes.InvalidArgument, "invalid path %q in field mask: no such field %q in message %s",
						pathstr, p, msg.FullName())
				}
				path = append(path, protopath.FieldAccess(field))
			}
		}

		rvs = append(rvs, s.Reactive(path))
	}

	if in.Bind {
		reactive.Bind(stream.Context(), func(v []protoreflect.Value) {
			items := make([]*ReactiveEvent, 0, len(v))
			for i, value := range v {
				items = append(items, &ReactiveEvent{
					Index: int32(i),
					Value: corev1.NewValue(value),
				})
			}
			stream.Send(&ReactiveEvents{Items: items})
		}, rvs...)
	} else {
		for i, rv := range rvs {
			i := i
			rv.WatchFunc(stream.Context(), func(value protoreflect.Value) {
				stream.Send(&ReactiveEvents{
					Items: []*ReactiveEvent{
						{
							Index: int32(i),
							Value: corev1.NewValue(value),
						},
					},
				})
			})
		}
	}

	return nil
}

var ProtoPath = (&GatewayConfigSpec{}).ProtoPath
