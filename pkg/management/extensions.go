package management

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	gsync "github.com/kralicky/gpkg/sync"
	"github.com/kralicky/grpc-gateway/v2/runtime"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
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
	Conn       *grpc.ClientConn
	InputType  *desc.MessageDescriptor
	OutputType *desc.MessageDescriptor
}

type StreamDirector func(ctx context.Context, fullMethodName string) (context.Context, *UnknownStreamMetadata, error)

func (m *Server) configureApiExtensionDirector(ctx waitctx.RestrictiveContext, pl plugins.LoaderInterface) StreamDirector {
	lg := m.logger
	methodTable := gsync.Map[string, *UnknownStreamMetadata]{}
	pl.Hook(hooks.OnLoadMC(func(p types.ManagementAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
		reflectClient := grpcreflect.NewClient(ctx, rpb.NewServerReflectionClient(cc))
		sd, err := p.Descriptor(ctx, &emptypb.Empty{})
		if err != nil {
			m.logger.With(
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
				Conn:       cc,
				InputType:  mtd.GetInputType(),
				OutputType: mtd.GetOutputType(),
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
		defer m.apiExtMu.Unlock()
		m.apiExtensions = append(m.apiExtensions, apiExtension{
			client:      client,
			clientConn:  cc,
			serviceDesc: svcDesc,
			httpRules:   httpRules,
		})
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

func (m *Server) configureHttpApiExtensions(mux *runtime.ServeMux) {
	lg := m.logger
	m.apiExtMu.RLock()
	defer m.apiExtMu.RUnlock()
	for _, ext := range m.apiExtensions {
		stub := grpcdynamic.NewStub(ext.clientConn)
		svcDesc := ext.serviceDesc
		for _, rule := range ext.httpRules {
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
			"method", rule.Method.GetName(),
			"path", path,
		)
		lg.Debug("handling http request")
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		methodDesc := svcDesc.FindMethodByName(rule.Method.GetName())
		_, outboundMarshaler := runtime.MarshalerForRequest(mux, req)

		rctx, err := runtime.AnnotateContext(ctx, mux, req, methodDesc.GetFullyQualifiedName(), runtime.WithHTTPPathPattern(path))
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}

		reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		} else if len(body) > 0 {
			if err := reqMsg.UnmarshalJSON(body); err != nil {
				lg.Error(err)
				runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
				return
			}
		}

		var metadata runtime.ServerMetadata
		resp, err := stub.InvokeRpc(rctx, methodDesc, reqMsg,
			grpc.Header(&metadata.HeaderMD), grpc.Trailer(&metadata.TrailerMD))
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}

		jsonData, err := resp.(*dynamic.Message).MarshalJSON()
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
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
		request := dynamic.NewMessage(meta.InputType)
		if err := stream.RecvMsg(request); err != nil {
			return err
		}
		reply := dynamic.NewMessage(meta.OutputType)
		err = meta.Conn.Invoke(outgoingCtx, fullMethodName, request, reply)
		if err != nil {
			return err
		}
		return stream.SendMsg(reply)
	}
}
