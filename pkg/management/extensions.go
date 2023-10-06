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
	"time"

	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/gorilla/websocket"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	gsync "github.com/kralicky/gpkg/sync"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
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
	Status          *health.ServingStatus
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

			client := apiextensions.NewManagementAPIExtensionClient(cc)
			healthChecker := health.ServiceHealthChecker{
				HealthCheckMethod: apiextensions.ManagementAPIExtension_CheckHealth_FullMethodName,
				Service:           svcName,
			}
			servingStatus := &health.ServingStatus{}
			if err := healthChecker.Start(ctx, cc, servingStatus.Set); err != nil {
				lg.With(
					zap.Error(err),
					zap.String("service", svcName),
				).Error("failed to start health checker for api extension service")
				return
			}

			for _, mtd := range svcDesc.GetMethods() {
				fullName := fmt.Sprintf("/%s/%s", svcName, mtd.GetName())
				lg.With(
					"name", fullName,
				).Info("loading method")

				methodTable.Store(fullName, &UnknownStreamMetadata{
					Conn:            cc,
					Status:          servingStatus,
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

			m.apiExtMu.Lock()
			m.apiExtensions = append(m.apiExtensions, apiExtension{
				client:      client,
				clientConn:  cc,
				status:      servingStatus,
				serviceDesc: svcDesc,
				httpRules:   httpRules,
			})
			m.apiExtMu.Unlock()
		}
	}))

	return func(ctx context.Context, fullMethodName string) (context.Context, *UnknownStreamMetadata, error) {
		if conn, ok := methodTable.Load(fullMethodName); ok {
			switch conn.Status.Get() {
			case healthpb.HealthCheckResponse_NOT_SERVING:
				return nil, nil, status.Error(codes.Unavailable, "service is not ready or has been shut down")
			case healthpb.HealthCheckResponse_SERVICE_UNKNOWN:
				return nil, nil, status.Error(codes.Internal, "unknown service")
			}
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

	status := &health.ServingStatus{} // local server, which is always serving
	status.Set(healthpb.HealthCheckResponse_SERVING)
	m.configureServiceStubHandlers(mux, stub, desc, rules, status)
	return nil
}

func (m *Server) configureHttpApiExtensions(mux *runtime.ServeMux) {
	m.apiExtMu.RLock()
	defer m.apiExtMu.RUnlock()
	for _, ext := range m.apiExtensions {
		stub := grpcdynamic.NewStub(ext.clientConn)
		svcDesc := ext.serviceDesc
		httpRules := ext.httpRules
		m.configureServiceStubHandlers(mux, stub, svcDesc, httpRules, ext.status)
	}
}

func (m *Server) configureServiceStubHandlers(
	mux *runtime.ServeMux,
	stub grpcdynamic.Stub,
	svcDesc *desc.ServiceDescriptor,
	httpRules []*managementv1.HTTPRuleDescriptor,
	svcStatus *health.ServingStatus,
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

		if err := mux.HandlePath(method, qualifiedPath, newHandler(stub, svcDesc, mux, rule, svcStatus, path)); err != nil {
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
	svcStatus *health.ServingStatus,
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
		inboundMarshaler, outboundMarshaler := runtime.MarshalerForRequest(mux, req)

		methodDesc := svcDesc.FindMethodByName(rule.Method.GetName())
		if methodDesc == nil {
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Errorf(codes.Unimplemented, "unknown method %q", rule.Method.GetName()))
			return
		}

		switch svcStatus.Get() {
		case healthpb.HealthCheckResponse_NOT_SERVING:
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Error(codes.Unavailable, "service is not ready or has been shut down"))
			return
		case healthpb.HealthCheckResponse_SERVICE_UNKNOWN:
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Error(codes.Internal, "unknown service"))
			return
		}

		ctx, err := runtime.AnnotateContext(ctx, mux, req, methodDesc.GetFullyQualifiedName(), runtime.WithHTTPPathPattern(path))
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err) // already a grpc error
			return
		}

		if methodDesc.IsClientStreaming() || methodDesc.IsServerStreaming() {
			if !websocket.IsWebSocketUpgrade(req) {
				w.WriteHeader(http.StatusUpgradeRequired)
				return
			}
			conn, err := (&websocket.Upgrader{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			}).Upgrade(w, req, http.Header{})
			if err != nil {
				lg.With(zap.Error(err)).Error("failed to upgrade connection")
				return
			}

			if err := handleWebsocketStream(ctx, conn, stub, methodDesc, lg); err != nil {
				lg.With(zap.Error(err)).Error("websocket stream error")
				// can't send http errors after the connection has been hijacked
			}
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
		} else if len(body) > 0 && reqMsg.GetMessageDescriptor().GetFullyQualifiedName() != "google.protobuf.Empty" {
			var err error
			bodyFieldPath := rule.GetHttp().GetBody()
			if bodyFieldPath == "" {
				bodyFieldPath = "*"
			}
			if bodyFieldPath == "*" {
				err = inboundMarshaler.Unmarshal(body, reqMsg)
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
					err = inboundMarshaler.Unmarshal(body, dynamicBodyField)
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
		resp, err := stub.InvokeRpc(ctx, methodDesc, reqMsg,
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
		respData, err := outboundMarshaler.Marshal(d)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to marshal response")

			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req,
				status.Errorf(codes.Internal, "internal error: failed to marshal response: %v", err))
			return
		}
		if _, err := w.Write(respData); err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to write response")
		}
	}
}

const (
	heartbeatTimeout  = 35 * time.Second
	heartbeatInterval = 11 * time.Second
)

func handleWebsocketStream(
	ctx context.Context,
	conn *websocket.Conn,
	stub grpcdynamic.Stub,
	methodDesc *desc.MethodDescriptor,
	lg *zap.SugaredLogger,
) error {
	isClientStreaming := methodDesc.IsClientStreaming()
	isServerStreaming := methodDesc.IsServerStreaming()
	if isClientStreaming && isServerStreaming {
		backendStream, err := stub.InvokeRpcBidiStream(ctx, methodDesc)
		if err != nil {
			lg.With(zap.Error(err)).Error("backend rpc error")
			return err
		}
		return handleWebsocketBidiStream(ctx, conn, backendStream, methodDesc)
	}

	if isServerStreaming {
		return handleWebsocketServerStream(ctx, conn, stub, methodDesc, lg)
	}
	backendStream, err := stub.InvokeRpcClientStream(ctx, methodDesc)
	if err != nil {
		lg.With(zap.Error(err)).Error("backend rpc error")
		return err
	}
	return handleWebsocketClientStream(ctx, conn, backendStream, methodDesc, lg)
}

func handleWebsocketClientStream(_ context.Context, _ *websocket.Conn, _ *grpcdynamic.ClientStream, _ *desc.MethodDescriptor, _ *zap.SugaredLogger) error {
	return fmt.Errorf("not implemented")
}

func handleWebsocketServerStream(
	ctx context.Context,
	conn *websocket.Conn,
	stub grpcdynamic.Stub,
	methodDesc *desc.MethodDescriptor,
	lg *zap.SugaredLogger,
) error {
	lg.Debug("handling websocket server stream")

	// messages from the backend to forward to the websocket
	send := make(chan *dynamic.Message, 1)
	// messages from the websocket to forward to the backend
	recv := make(chan *dynamic.Message, 1)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var backendStream *grpcdynamic.ServerStream
		select {
		case <-ctx.Done():
			lg.Debug("socket closed before initial request was received")
			return ctx.Err()
		case msg := <-recv:
			var err error
			backendStream, err = stub.InvokeRpcServerStream(ctx, methodDesc, msg)
			if err != nil {
				return fmt.Errorf("failed to invoke backend stream: %w", err)
			}
		}
		for ctx.Err() == nil {
			resp, err := backendStream.RecvMsg()
			if err != nil {
				return err
			}
			dm, err := dynamic.AsDynamicMessage(resp)
			if err != nil {
				lg.With(zap.Error(err)).Warn("could not convert message to dynamic message")
				continue
			}
			send <- dm
		}
		return ctx.Err()
	})

	eg.Go(func() error {
		var recvOnce bool
		for ctx.Err() == nil {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				lg.With(zap.Error(err)).Error("error reading from websocket")
				return err
			}
			switch msgType {
			case websocket.CloseMessage:
				return io.EOF
			case websocket.BinaryMessage:
				if !recvOnce {
					recvOnce = true
				} else {
					lg.Warn("skipping additional messages in server stream")
					continue
				}
				reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
				if err := reqMsg.Unmarshal(data); err != nil {
					return fmt.Errorf("failed to unmarshal websocket message: %w", err)
				}
				recv <- reqMsg
			}
		}
		return ctx.Err()
	})

	timeout := time.NewTimer(heartbeatTimeout)
	heartbeat := time.NewTicker(heartbeatInterval)

	defer heartbeat.Stop()
	defer timeout.Stop()
	conn.SetPongHandler(func(appData string) error {
		if timeout.Stop() {
			timeout.Reset(heartbeatTimeout)
		}
		return nil
	})

	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case msg := <-send:
				data, err := msg.Marshal()
				if err != nil {
					return err
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return err
				}
			case <-heartbeat.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return err
				}
			case <-timeout.C:
				return fmt.Errorf("socket timed out after %v", heartbeatTimeout)
			}
		}
	})

	err := eg.Wait()
	if err != nil {
		lg.With(zap.Error(err)).Error("error occurred handling websocket")
	}
	if err := conn.Close(); err != nil {
		lg.With(zap.Error(err)).Error("error occurred while closing websocket")
	}
	return err
}

func handleWebsocketBidiStream(_ context.Context, _ *websocket.Conn, _ *grpcdynamic.BidiStream, _ *desc.MethodDescriptor) error {
	return fmt.Errorf("not implemented")
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
