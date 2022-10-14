// Code generated by protoc-gen-grpc-gateway. DO NOT EDIT.
// source: github.com/rancher/opni/plugins/pkg/apis/server/log/alerting.log.proto

/*
Package log is a reverse proxy.

It translates gRPC into RESTful JSON APIs.
*/
package log

import (
	"context"
	"io"
	"net/http"

	"github.com/kralicky/grpc-gateway/v2/runtime"
	"github.com/kralicky/grpc-gateway/v2/utilities"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Suppress "imported and not used" errors
var _ codes.Code
var _ io.Reader
var _ status.Status
var _ = runtime.String
var _ = utilities.NewDoubleArray
var _ = metadata.Join

var (
	filter_AlertLogs_ListAlertLogs_0 = &utilities.DoubleArray{Encoding: map[string]int{}, Base: []int(nil), Check: []int(nil)}
)

func request_AlertLogs_ListAlertLogs_0(ctx context.Context, marshaler runtime.Marshaler, client AlertLogsClient, req *http.Request, pathParams map[string]string) (proto.Message, runtime.ServerMetadata, error) {
	var protoReq common.ListAlertLogRequest
	var metadata runtime.ServerMetadata

	if err := req.ParseForm(); err != nil {
		return nil, metadata, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := runtime.PopulateQueryParameters(&protoReq, req.Form, filter_AlertLogs_ListAlertLogs_0); err != nil {
		return nil, metadata, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	msg, err := client.ListAlertLogs(ctx, &protoReq, grpc.Header(&metadata.HeaderMD), grpc.Trailer(&metadata.TrailerMD))
	return msg, metadata, err

}

func local_request_AlertLogs_ListAlertLogs_0(ctx context.Context, marshaler runtime.Marshaler, server AlertLogsServer, req *http.Request, pathParams map[string]string) (proto.Message, runtime.ServerMetadata, error) {
	var protoReq common.ListAlertLogRequest
	var metadata runtime.ServerMetadata

	if err := req.ParseForm(); err != nil {
		return nil, metadata, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := runtime.PopulateQueryParameters(&protoReq, req.Form, filter_AlertLogs_ListAlertLogs_0); err != nil {
		return nil, metadata, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	msg, err := server.ListAlertLogs(ctx, &protoReq)
	return msg, metadata, err

}

// RegisterAlertLogsHandlerServer registers the http handlers for service AlertLogs to "mux".
// UnaryRPC     :call AlertLogsServer directly.
// StreamingRPC :currently unsupported pending https://github.com/grpc/grpc-go/issues/906.
// Note that using this registration option will cause many gRPC library features to stop working. Consider using RegisterAlertLogsHandlerFromEndpoint instead.
func RegisterAlertLogsHandlerServer(ctx context.Context, mux *runtime.ServeMux, server AlertLogsServer) error {

	mux.Handle("GET", pattern_AlertLogs_ListAlertLogs_0, func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		var stream runtime.ServerTransportStream
		ctx = grpc.NewContextWithServerTransportStream(ctx, &stream)
		inboundMarshaler, outboundMarshaler := runtime.MarshalerForRequest(mux, req)
		var err error
		var annotatedContext context.Context
		annotatedContext, err = runtime.AnnotateIncomingContext(ctx, mux, req, "/alerting.log.AlertLogs/ListAlertLogs", runtime.WithHTTPPathPattern("/events"))
		if err != nil {
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}
		resp, md, err := local_request_AlertLogs_ListAlertLogs_0(annotatedContext, inboundMarshaler, server, req, pathParams)
		md.HeaderMD, md.TrailerMD = metadata.Join(md.HeaderMD, stream.Header()), metadata.Join(md.TrailerMD, stream.Trailer())
		annotatedContext = runtime.NewServerMetadataContext(annotatedContext, md)
		if err != nil {
			runtime.HTTPError(annotatedContext, mux, outboundMarshaler, w, req, err)
			return
		}

		forward_AlertLogs_ListAlertLogs_0(annotatedContext, mux, outboundMarshaler, w, req, resp, mux.GetForwardResponseOptions()...)

	})

	return nil
}

// RegisterAlertLogsHandlerFromEndpoint is same as RegisterAlertLogsHandler but
// automatically dials to "endpoint" and closes the connection when "ctx" gets done.
func RegisterAlertLogsHandlerFromEndpoint(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error) {
	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if cerr := conn.Close(); cerr != nil {
				grpclog.Infof("Failed to close conn to %s: %v", endpoint, cerr)
			}
			return
		}
		go func() {
			<-ctx.Done()
			if cerr := conn.Close(); cerr != nil {
				grpclog.Infof("Failed to close conn to %s: %v", endpoint, cerr)
			}
		}()
	}()

	return RegisterAlertLogsHandler(ctx, mux, conn)
}

// RegisterAlertLogsHandler registers the http handlers for service AlertLogs to "mux".
// The handlers forward requests to the grpc endpoint over "conn".
func RegisterAlertLogsHandler(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
	return RegisterAlertLogsHandlerClient(ctx, mux, NewAlertLogsClient(conn))
}

// RegisterAlertLogsHandlerClient registers the http handlers for service AlertLogs
// to "mux". The handlers forward requests to the grpc endpoint over the given implementation of "AlertLogsClient".
// Note: the gRPC framework executes interceptors within the gRPC handler. If the passed in "AlertLogsClient"
// doesn't go through the normal gRPC flow (creating a gRPC client etc.) then it will be up to the passed in
// "AlertLogsClient" to call the correct interceptors.
func RegisterAlertLogsHandlerClient(ctx context.Context, mux *runtime.ServeMux, client AlertLogsClient) error {

	mux.Handle("GET", pattern_AlertLogs_ListAlertLogs_0, func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		inboundMarshaler, outboundMarshaler := runtime.MarshalerForRequest(mux, req)
		var err error
		var annotatedContext context.Context
		annotatedContext, err = runtime.AnnotateContext(ctx, mux, req, "/alerting.log.AlertLogs/ListAlertLogs", runtime.WithHTTPPathPattern("/events"))
		if err != nil {
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}
		resp, md, err := request_AlertLogs_ListAlertLogs_0(annotatedContext, inboundMarshaler, client, req, pathParams)
		annotatedContext = runtime.NewServerMetadataContext(annotatedContext, md)
		if err != nil {
			runtime.HTTPError(annotatedContext, mux, outboundMarshaler, w, req, err)
			return
		}

		forward_AlertLogs_ListAlertLogs_0(annotatedContext, mux, outboundMarshaler, w, req, resp, mux.GetForwardResponseOptions()...)

	})

	return nil
}

var (
	pattern_AlertLogs_ListAlertLogs_0 = runtime.MustPattern(runtime.NewPattern(1, []int{2, 0}, []string{"events"}, ""))
)

var (
	forward_AlertLogs_ListAlertLogs_0 = runtime.ForwardResponseMessage
)
