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
	"github.com/kralicky/grpc-gateway/v2/runtime"
	"github.com/mwitkow/grpc-proxy/proxy"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) APIExtensions(context.Context, *emptypb.Empty) (*APIExtensionInfoList, error) {
	resp := &APIExtensionInfoList{}
	for _, ext := range m.apiExtensions {
		resp.Items = append(resp.Items, &APIExtensionInfo{
			ServiceDesc: ext.serviceDesc.AsServiceDescriptorProto(),
			Rules:       ext.httpRules,
		})
	}
	return resp, nil
}

func (m *Server) configureApiExtensionDirector(ctx context.Context) proxy.StreamDirector {
	lg := m.logger
	lg.Infof("loading api extensions from %d plugins", len(m.apiExtPlugins))
	methodTable := map[string]*grpc.ClientConn{}
	for _, plugin := range m.apiExtPlugins {
		reflectClient := grpcreflect.NewClient(ctx,
			rpb.NewServerReflectionClient(plugin.Client))
		sd, err := plugin.Typed.Descriptor(ctx, &emptypb.Empty{})
		if err != nil {
			m.logger.With(
				zap.Error(err),
				zap.String("plugin", plugin.Metadata.Module),
			).Error("failed to get extension descriptor")
			continue
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
			continue
		}
		svcName := sd.GetName()
		lg.With(
			"name", svcName,
		).Info("loading service")
		svcMethods := sd.GetMethod()
		for _, mtd := range svcMethods {
			fullName := fmt.Sprintf("/%s/%s", svcName, mtd.GetName())
			lg.With(
				"name", fullName,
			).Info("loading method")
			methodTable[fullName] = plugin.Client
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
		m.apiExtensions = append(m.apiExtensions, apiExtension{
			client:      plugin.Typed,
			clientConn:  plugin.Client,
			serviceDesc: svcDesc,
			httpRules:   httpRules,
		})
	}
	if len(methodTable) == 0 {
		lg.Info("no api extensions found")
	}

	return func(ctx context.Context, fullMethodName string) (context.Context, *grpc.ClientConn, error) {
		if conn, ok := methodTable[fullMethodName]; ok {
			return ctx, conn, nil
		}
		return nil, nil, status.Errorf(codes.Unimplemented, "unknown method %q", fullMethodName)
	}
}

func (m *Server) configureHttpApiExtensions(mux *runtime.ServeMux) {
	lg := m.logger
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

func loadHttpRuleDescriptors(svc *desc.ServiceDescriptor) []*HTTPRuleDescriptor {
	httpRule := []*HTTPRuleDescriptor{}
	for _, method := range svc.GetMethods() {
		protoMethodDesc := method.AsMethodDescriptorProto()
		protoOptions := protoMethodDesc.GetOptions()
		if proto.HasExtension(protoOptions, annotations.E_Http) {
			ext := proto.GetExtension(protoOptions, annotations.E_Http)
			rule, ok := ext.(*annotations.HttpRule)
			if !ok {
				continue
			}
			httpRule = append(httpRule, &HTTPRuleDescriptor{
				Http:   rule,
				Method: protoMethodDesc,
			})
			for _, additionalRule := range rule.AdditionalBindings {
				httpRule = append(httpRule, &HTTPRuleDescriptor{
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
	rule *HTTPRuleDescriptor,
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
