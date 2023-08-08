package management

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"reflect"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	gsync "github.com/kralicky/gpkg/sync"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/api/annotations"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/util"
)

func (m *Server) APIExtensions(context.Context, *emptypb.Empty) (*managementv1.APIExtensionInfoList, error) {
	m.apiExtMu.RLock()
	defer m.apiExtMu.RUnlock()
	resp := &managementv1.APIExtensionInfoList{}
	for _, ext := range m.apiExtensions {
		resp.Items = append(resp.Items, &managementv1.APIExtensionInfo{
			ServiceDesc: ext.serviceDesc.AsServiceDescriptorProto(),
			Rules:       ext.httpRules,
		})
	}
	return resp, nil
}

type UnknownStreamMetadata struct {
	Conn            *grpc.ClientConn
	InputType       *desc.MessageDescriptor
	OutputType      *desc.MessageDescriptor
	ServerStreaming bool
	ClientStreaming bool
}

type StreamDirector func(ctx context.Context, fullMethodName string) (context.Context, *UnknownStreamMetadata, error)

func (m *Server) configureApiExtensionDirector(ctx context.Context, pl plugins.LoaderInterface) StreamDirector {
	lg := m.logger
	methodTable := gsync.Map[string, *UnknownStreamMetadata]{}
	pl.Hook(hooks.OnLoadMC(func(p types.ManagementAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
		reflectClient := grpcreflect.NewClientAuto(ctx, cc)
		sds, err := p.Descriptors(ctx, &emptypb.Empty{})
		if err != nil {
			m.logger.With(
				zap.Error(err),
				zap.String("plugin", md.Module),
			).Error("failed to get extension descriptors")
			return
		}
		for _, sd := range sds.Items {
			lg.Info("got extension descriptor for service " + sd.GetName())

			svcDesc, err := reflectClient.ResolveService(sd.GetName())
			if err != nil {
				m.logger.With(
					zap.Error(err),
				).Error("failed to resolve extension service")
				return
			}

			svcName := sd.GetName()
			lg.With(
				"name", svcName,
			).Info("loading service")
			for _, mtd := range svcDesc.GetMethods() {
				fullName := fmt.Sprintf("/%s/%s", svcName, mtd.GetName())
				lg.With(
					"name", fullName,
				).Info("loading method")

				methodTable.Store(fullName, &UnknownStreamMetadata{
					Conn:            cc,
					InputType:       mtd.GetInputType(),
					OutputType:      mtd.GetOutputType(),
					ServerStreaming: mtd.IsServerStreaming(),
					ClientStreaming: mtd.IsClientStreaming(),
				})
			}
			httpRules := loadHttpRuleDescriptors(svcDesc)
			if len(httpRules) > 0 {
				lg.With(
					"name", svcName,
					"rules", len(httpRules),
				).Info("loading http rules")
				lg.With(
					"name", svcName,
					"rules", httpRules,
				).Debug("rule descriptors")
			} else {
				lg.With(
					"name", svcName,
				).Info("service has no http rules")
			}

			client := apiextensions.NewManagementAPIExtensionClient(cc)
			m.apiExtMu.Lock()
			m.apiExtensions = append(m.apiExtensions, apiExtension{
				client:      client,
				clientConn:  cc,
				serviceDesc: svcDesc,
				httpRules:   httpRules,
			})
			m.apiExtMu.Unlock()
		}
	}))

	return func(ctx context.Context, fullMethodName string) (context.Context, *UnknownStreamMetadata, error) {
		if conn, ok := methodTable.Load(fullMethodName); ok {
			md, _ := metadata.FromIncomingContext(ctx)
			ctx = metadata.NewOutgoingContext(ctx, md.Copy())
			return ctx, conn, nil
		}
		return nil, nil, status.Errorf(codes.Unimplemented, "unknown method %q", fullMethodName)
	}
}

func (m *Server) configureManagementHttpApi(ctx context.Context, mux *runtime.ServeMux) error {
	m.apiExtMu.RLock()
	defer m.apiExtMu.RUnlock()

	cc, err := grpc.DialContext(ctx, m.config.GRPCListenAddress,
		grpc.WithBlock(),
		grpc.WithContextDialer(util.DialProtocol),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to dial grpc server: %w", err)
	}
	stub := grpcdynamic.NewStub(cc)

	desc, err := grpcreflect.LoadServiceDescriptor(&managementv1.Management_ServiceDesc)
	if err != nil {
		return fmt.Errorf("failed to load service descriptor: %w", err)
	}

	rules := loadHttpRuleDescriptors(desc)

	m.configureServiceStubHandlers(mux, stub, desc, rules)
	return nil
}

func (m *Server) configureHttpApiExtensions(mux *runtime.ServeMux) {
	m.apiExtMu.RLock()
	defer m.apiExtMu.RUnlock()
	for _, ext := range m.apiExtensions {
		stub := grpcdynamic.NewStub(ext.clientConn)
		svcDesc := ext.serviceDesc
		httpRules := ext.httpRules
		m.configureServiceStubHandlers(mux, stub, svcDesc, httpRules)
	}
}
func (m *Server) configureServiceStubHandlers(
	mux *runtime.ServeMux,
	stub grpcdynamic.Stub,
	svcDesc *desc.ServiceDescriptor,
	httpRules []*managementv1.HTTPRuleDescriptor,
) {
	lg := m.logger
	for _, rule := range httpRules {
		var method string
		var path string
		switch rp := rule.Http.Pattern.(type) {
		case *annotations.HttpRule_Get:
			method = "GET"
			path = rp.Get
		case *annotations.HttpRule_Post:
			method = "POST"
			path = rp.Post
		case *annotations.HttpRule_Put:
			method = "PUT"
			path = rp.Put
		case *annotations.HttpRule_Delete:
			method = "DELETE"
			path = rp.Delete
		case *annotations.HttpRule_Patch:
			method = "PATCH"
			path = rp.Patch
		}
		qualifiedPath := fmt.Sprintf("/%s%s", svcDesc.GetName(), path)
		if err := mux.HandlePath(method, qualifiedPath, newHandler(stub, svcDesc, mux, rule, path)); err != nil {
			lg.With(
				zap.Error(err),
				zap.String("method", method),
				zap.String("path", qualifiedPath),
			).Error("failed to configure http handler")
		} else {
			lg.With(
				zap.String("method", method),
				zap.String("path", qualifiedPath),
			).Debug("configured http handler")
		}
	}
}

type statusOnlyMarshaler struct {
	runtime.Marshaler
}

func (m statusOnlyMarshaler) Marshal(v any) ([]byte, error) {
	if st, ok := v.(*statuspb.Status); ok {
		return []byte(st.GetMessage()), nil
	}
	return m.Marshaler.Marshal(v)
}

func extensionsErrorHandler(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	runtime.DefaultHTTPErrorHandler(ctx, mux, statusOnlyMarshaler{
		Marshaler: marshaler,
	}, w, r, err)
}

func loadHttpRuleDescriptors(svc *desc.ServiceDescriptor) []*managementv1.HTTPRuleDescriptor {
	httpRule := []*managementv1.HTTPRuleDescriptor{}
	for _, method := range svc.GetMethods() {
		protoMethodDesc := method.AsMethodDescriptorProto()
		protoOptions := protoMethodDesc.GetOptions()
		if proto.HasExtension(protoOptions, annotations.E_Http) {
			ext := proto.GetExtension(protoOptions, annotations.E_Http)
			rule, ok := ext.(*annotations.HttpRule)
			if !ok {
				continue
			}
			httpRule = append(httpRule, &managementv1.HTTPRuleDescriptor{
				Http:   rule,
				Method: protoMethodDesc,
			})
			for _, additionalRule := range rule.AdditionalBindings {
				httpRule = append(httpRule, &managementv1.HTTPRuleDescriptor{
					Http:   additionalRule,
					Method: protoMethodDesc,
				})
			}
		}
	}
	return httpRule
}

func newHandler(
	stub grpcdynamic.Stub,
	svcDesc *desc.ServiceDescriptor,
	mux *runtime.ServeMux,
	rule *managementv1.HTTPRuleDescriptor,
	path string,
) runtime.HandlerFunc {
	lg := logger.New().Named("apiext")
	return func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		lg := lg.With(
			"svc", svcDesc.GetName(),
			"method", rule.Method.GetName(),
			"path", path,
		)
		lg.Debug("handling http request")
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		_, outboundMarshaler := runtime.MarshalerForRequest(mux, req)

		methodDesc := svcDesc.FindMethodByName(rule.Method.GetName())
		if methodDesc == nil {
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Errorf(codes.Unimplemented, "unknown method %q", rule.Method.GetName()))
			return
		}

		rctx, err := runtime.AnnotateContext(ctx, mux, req, methodDesc.GetFullyQualifiedName(), runtime.WithHTTPPathPattern(path))
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err) // already a grpc error
			return
		}

		reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
		for k, v := range pathParams {
			if err := decodeAndSetField(reqMsg, k, v); err != nil {
				lg.With(
					"key", k,
					"value", v,
					zap.Error(err),
				).Error("failed to decode path parameter")
				runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
					status.Errorf(codes.InvalidArgument, err.Error()))
				return
			}
		}
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Errorf(codes.InvalidArgument, err.Error()))
			return
		} else if len(body) > 0 {
			var err error
			bodyFieldPath := rule.GetHttp().GetBody()
			if bodyFieldPath == "" {
				bodyFieldPath = "*"
			}
			if bodyFieldPath == "*" {
				err = reqMsg.UnmarshalMergeJSON(body)
			} else {
				// unmarshal the request into a specific field
				fd := reqMsg.FindFieldDescriptorByJSONName(bodyFieldPath)
				if fd == nil {
					runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
						status.Errorf(codes.InvalidArgument, "unknown field %q", bodyFieldPath))
					return
				}
				if fd.GetType() == dpb.FieldDescriptorProto_TYPE_MESSAGE {
					bodyField := reqMsg.GetField(fd)
					var dynamicBodyField *dynamic.Message
					if reflect.ValueOf(bodyField).IsNil() {
						dynamicBodyField = dynamic.NewMessage(fd.GetMessageType())
						reqMsg.SetField(fd, dynamicBodyField)
					} else {
						var ok bool
						dynamicBodyField, ok = bodyField.(*dynamic.Message)
						if !ok {
							panic("bug: request is not a *dynamic.Message")
						}
					}
					err = dynamicBodyField.UnmarshalMergeJSON(body)
				} else {
					if err := decodeAndSetField(reqMsg, bodyFieldPath, string(body)); err != nil {
						if _, ok := status.FromError(err); ok {
							runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
						} else {
							runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
								status.Errorf(codes.InvalidArgument, "failed to decode body field %q: %v", bodyFieldPath, err))
						}
						return
					}
				}
			}
			if err != nil {
				lg.With(
					zap.Error(err),
					zap.String("body", string(body)),
				).Errorf("failed to unmarshal request body into %q", bodyFieldPath)
				// special case here, make empty string errors more obvious
				if strings.HasSuffix(err.Error(), "named ") { // note the trailing space
					err = fmt.Errorf("%w(empty)", err)
				}
				runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
					status.Errorf(codes.InvalidArgument, "failed to unmarshal request body: %v", err),
				)
				return
			}
		}

		var metadata runtime.ServerMetadata
		resp, err := stub.InvokeRpc(rctx, methodDesc, reqMsg,
			grpc.Header(&metadata.HeaderMD), grpc.Trailer(&metadata.TrailerMD))
		if err != nil {
			lg.With(
				zap.Error(err),
			).Debug("rpc error")
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}
		d, err := dynamic.AsDynamicMessage(resp)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("bad response message")
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Errorf(codes.Internal, "internal error: bad response message: %v", err))
		}
		jsonData, err := d.MarshalJSONPB(&jsonpb.Marshaler{
			EmitDefaults: true,
			EnumsAsInts:  true,
		})
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to marshal response")

			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Errorf(codes.Internal, "internal error: failed to marshal response: %v", err))
			return
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		if _, err := w.Write(jsonData); err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to write response")
		}
	}
}

func unknownServiceHandler(director StreamDirector) grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		fullMethodName, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return status.Errorf(codes.Internal, "method does not exist")
		}
		outgoingCtx, meta, err := director(stream.Context(), fullMethodName)
		if err != nil {
			return err
		}

		switch {
		case meta.ServerStreaming && !meta.ClientStreaming:
			request := dynamic.NewMessage(meta.InputType)
			if err := stream.RecvMsg(request); err != nil {
				return err
			}
			cs, err := meta.Conn.NewStream(outgoingCtx, &grpc.StreamDesc{
				StreamName:    path.Base(fullMethodName),
				ServerStreams: true,
				ClientStreams: false,
			}, fullMethodName)
			if err != nil {
				return err
			}

			if err := cs.SendMsg(request); err != nil {
				return err
			}

			if md, err := cs.Header(); err == nil {
				stream.SendHeader(md)
			}

			if err := cs.CloseSend(); err != nil {
				return err
			}

			for {
				reply := dynamic.NewMessage(meta.OutputType)
				if err := cs.RecvMsg(reply); err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				if err := stream.SendMsg(reply); err != nil {
					return err
				}
			}

			if t := cs.Trailer(); t != nil {
				stream.SetTrailer(t)
			}

			return nil
		case !meta.ServerStreaming && meta.ClientStreaming:
			cs, err := meta.Conn.NewStream(outgoingCtx, &grpc.StreamDesc{
				StreamName:    path.Base(fullMethodName),
				ServerStreams: false,
				ClientStreams: true,
			}, fullMethodName)
			if err != nil {
				return err
			}

			if md, err := cs.Header(); err == nil {
				stream.SendHeader(md)
			}

			for {
				toSend := dynamic.NewMessage(meta.InputType)
				if err := stream.RecvMsg(toSend); err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return err
				}
				if err := cs.SendMsg(toSend); err != nil {
					return err
				}
			}
			if err := cs.CloseSend(); err != nil {
				return err
			}

			reply := dynamic.NewMessage(meta.OutputType)
			if err := cs.RecvMsg(reply); err != nil {
				return err
			}
			if err := stream.SendMsg(reply); err != nil {
				return err
			}

			if t := cs.Trailer(); t != nil {
				stream.SetTrailer(t)
			}

			return nil
		case meta.ServerStreaming && meta.ClientStreaming:
			cs, err := meta.Conn.NewStream(outgoingCtx, &grpc.StreamDesc{
				StreamName:    path.Base(fullMethodName),
				ServerStreams: true,
				ClientStreams: true,
			}, fullMethodName)
			if err != nil {
				return err
			}

			if md, err := cs.Header(); err == nil {
				stream.SendHeader(md)
			}
			var eg errgroup.Group
			// client -> server
			eg.Go(func() error {
				for {
					reply := dynamic.NewMessage(meta.OutputType)
					if err := cs.RecvMsg(reply); err != nil {
						return err
					}
					if err := stream.SendMsg(reply); err != nil {
						return err
					}
				}
			})

			// server -> client
			eg.Go(func() error {
				for {
					toSend := dynamic.NewMessage(meta.InputType)
					if err := stream.RecvMsg(toSend); err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						return err
					}
					if err := cs.SendMsg(toSend); err != nil {
						return err
					}
				}
				if err := cs.CloseSend(); err != nil {
					return err
				}

				return io.EOF
			})
			err = eg.Wait()
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}

			if t := cs.Trailer(); t != nil {
				stream.SetTrailer(t)
			}
			return nil

		case !meta.ServerStreaming && !meta.ClientStreaming:
			fallthrough
		default:
			request := dynamic.NewMessage(meta.InputType)
			if err := stream.RecvMsg(request); err != nil {
				return err
			}
			reply := dynamic.NewMessage(meta.OutputType)
			var trailer metadata.MD
			err = meta.Conn.Invoke(outgoingCtx, fullMethodName, request, reply, grpc.Trailer(&trailer))
			if err != nil {
				return err
			}
			if trailer != nil {
				stream.SetTrailer(trailer)
			}
			return stream.SendMsg(reply)
		}
	}
}

func decodeAndSetField(reqMsg *dynamic.Message, k, v string) error {
	fieldPaths := strings.Split(k, ".")
	if len(fieldPaths) == 0 {
		return fmt.Errorf("field path %q is invalid", k)
	}
	var fd *desc.FieldDescriptor
	containingMsg := reqMsg
	for i, fp := range fieldPaths {
		fd = containingMsg.FindFieldDescriptorByJSONName(fp)
		if fd == nil {
			return fmt.Errorf("field path %q is invalid: message %q does not have field %q", k, containingMsg.GetMessageDescriptor().GetName(), fp)
		}
		// each field path must be a message, except for the last one
		if fd.GetType() != dpb.FieldDescriptorProto_TYPE_MESSAGE {
			if i == len(fieldPaths)-1 {
				fd = containingMsg.FindFieldDescriptorByJSONName(fp)
				break
			}
			return fmt.Errorf("field path %q is invalid: %q is not a message", k, fd.GetFullyQualifiedName())
		}
		msg := fd.GetMessageType()
		var subMsg *dynamic.Message
		if fld := containingMsg.GetField(fd); reflect.ValueOf(fld).IsNil() {
			subMsg = dynamic.NewMessage(msg)
			containingMsg.SetField(fd, subMsg)
		} else {
			var ok bool
			subMsg, ok = fld.(*dynamic.Message)
			if !ok {
				panic("bug: request is not a *dynamic.Message")
			}
		}
		containingMsg = subMsg
	}
	if fd == nil {
		return fmt.Errorf("field path %q is invalid", k)
	}

	var decoded any
	var err error
	switch fd.GetType() {
	case dpb.FieldDescriptorProto_TYPE_DOUBLE:
		decoded, err = runtime.Float64(v)
	case dpb.FieldDescriptorProto_TYPE_FLOAT:
		decoded, err = runtime.Float32(v)
	case dpb.FieldDescriptorProto_TYPE_INT64:
		decoded, err = runtime.Int64(v)
	case dpb.FieldDescriptorProto_TYPE_UINT64:
		decoded, err = runtime.Uint64(v)
	case dpb.FieldDescriptorProto_TYPE_INT32:
		decoded, err = runtime.Int32(v)
	case dpb.FieldDescriptorProto_TYPE_FIXED64:
		decoded, err = runtime.Uint64(v)
	case dpb.FieldDescriptorProto_TYPE_FIXED32:
		decoded, err = runtime.Uint32(v)
	case dpb.FieldDescriptorProto_TYPE_BOOL:
		decoded, err = runtime.Bool(v)
	case dpb.FieldDescriptorProto_TYPE_STRING:
		decoded, err = runtime.String(v)
	case dpb.FieldDescriptorProto_TYPE_GROUP, dpb.FieldDescriptorProto_TYPE_MESSAGE:
		return status.Errorf(codes.Unimplemented, "field %s of type %s is not supported", k, fd.GetType().String())
	case dpb.FieldDescriptorProto_TYPE_BYTES:
		decoded, err = runtime.Bytes(v)
	case dpb.FieldDescriptorProto_TYPE_UINT32:
		decoded, err = runtime.Uint32(v)
	case dpb.FieldDescriptorProto_TYPE_ENUM:
		enumType := fd.GetEnumType()
		if enumType == nil {
			err = fmt.Errorf("unknown enum type for field %s", k)
			break
		}
		vd := enumType.FindValueByName(v)
		if vd == nil {
			err = fmt.Errorf("enum %q does not have value %q", enumType.GetName(), v)
			break
		}
		decoded = vd.GetNumber()
	case dpb.FieldDescriptorProto_TYPE_SFIXED32:
		decoded, err = runtime.Int32(v)
	case dpb.FieldDescriptorProto_TYPE_SFIXED64:
		decoded, err = runtime.Int64(v)
	case dpb.FieldDescriptorProto_TYPE_SINT32:
		decoded, err = runtime.Int32(v)
	case dpb.FieldDescriptorProto_TYPE_SINT64:
		decoded, err = runtime.Int64(v)
	}
	if err != nil {
		shortTypeName := strings.ToLower(strings.TrimPrefix(fd.GetType().String(), "TYPE_"))
		return fmt.Errorf("failed to decode value for field %q: could not convert %q to type %s: %w", k, v, shortTypeName, err)
	}
	if err := containingMsg.TrySetField(fd, decoded); err != nil {
		return fmt.Errorf("failed to set field %q to value %q: %w", k, v, err)
	}
	return nil
}
