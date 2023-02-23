package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/runtime/protoimpl"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/storage"
	"github.com/zeebo/xxh3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	// headers
	CacheTypeClient = "client"
	CacheTypeServer = "server"
	// values

	GrpcNoCache = "no-cache"
	// server can also manually set this to revalidate the cache
	GrpcMustRevalidateCache = "must-revalidate"
	GrpcIgnoreCustomKeys    = "ignore-cache-keyer"
	GrpcCacheMiss           = "miss"
	GrpcCacheHit            = "hit"

	GrpcCacheIdentifier = "x-cache-req-id"
)

var (
	GrpcCacheControlHeader = func(cacheType string) string {
		return "cache-control-" + cacheType
	}
	GrpcCacheStatusHeader = func(fullMethod string) string {
		return "x-cache-" + fullMethod
	}

	GrpcMaxAge = func(d time.Duration) string {
		return fmt.Sprintf("max-age=%d", int(d.Seconds()))
	}
)

type ServicePackInterface interface {
	Unpack() (*grpc.ServiceDesc, any)
}

type ServicePack[T any] struct {
	desc *grpc.ServiceDesc
	impl T
}

func (s ServicePack[T]) Unpack() (*grpc.ServiceDesc, any) {
	return s.desc, s.impl
}

func PackService[T any](desc *grpc.ServiceDesc, impl T) ServicePack[T] {
	return ServicePack[T]{
		desc: desc,
		impl: impl,
	}
}

func StatusError(code codes.Code) error {
	return status.Error(code, code.String())
}

// Like status.Code(), but supports wrapped errors.
func StatusCode(err error) codes.Code {
	var grpcStatus interface{ GRPCStatus() *status.Status }
	code := codes.Unknown
	if errors.As(err, &grpcStatus) {
		code = grpcStatus.GRPCStatus().Code()
	}
	return code
}

func hash(method string, req proto.Message) string {
	return HashStrings([]string{method, string(Must(proto.MarshalOptions{
		Deterministic: true,
	}.Marshal(req)))})
}

func WithGrpcClientCaching(ctx context.Context, d time.Duration) context.Context {
	return WithCacheControlHeaders(ctx, CacheTypeClient, GrpcMaxAge(d))
}

func WithIgnoreServerCacheKeys(ctx context.Context) context.Context {
	return WithCacheControlHeaders(ctx, CacheTypeClient, GrpcIgnoreCustomKeys)
}

func WithBypassCache(ctx context.Context) context.Context {
	return WithCacheControlHeaders(
		WithCacheControlHeaders(ctx, CacheTypeClient, GrpcNoCache),
		CacheTypeServer,
		GrpcNoCache,
	)
}

func WithCacheControlHeaders(
	ctx context.Context,
	cacheType string,
	values ...string,
) context.Context {
	pairs := []string{}
	for _, v := range values {
		pairs = append(pairs, GrpcCacheControlHeader(cacheType), v)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func cacheMiss(cacheType string) metadata.MD {
	return metadata.Pairs(GrpcCacheStatusHeader(cacheType), GrpcCacheMiss)
}

func cacheHit(cacheType string) metadata.MD {
	return metadata.Pairs(GrpcCacheStatusHeader(cacheType), GrpcCacheHit)
}

// ForceClientCaching The client calling this RPC will be forced into caching the response
// for the given TTL, unless the client has explicitly opted-out of caching.
func ForceClientCaching(ctx context.Context, ttl time.Duration) {
	method, ok := grpc.Method(ctx)
	if !ok {
		// not grpc server-side
		return
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(map[string]string{})
	}
	md.Append(GrpcCacheStatusHeader(CacheTypeClient), GrpcCacheMiss)
	// need to uniquely identify who is requesting to cache the request
	md.Append(GrpcCacheControlHeader(CacheTypeClient), GrpcMaxAge(ttl))
	md.Append(GrpcCacheControlHeader(CacheTypeClient), GrpcMustRevalidateCache)
	setCacheRequestMd(md, method)
	grpc.SetHeader(ctx, md)
}

func ignoreCustomKeys(md metadata.MD, cacheType string) bool {
	for _, v := range md.Get(GrpcCacheControlHeader(cacheType)) {
		if v == GrpcIgnoreCustomKeys {
			return true
		}
	}
	return false
}

func noCache(md metadata.MD, cacheType string) bool {
	for _, v := range md.Get(GrpcCacheControlHeader(cacheType)) {
		if v == GrpcNoCache { // always override
			return true
		}
	}
	return false
}

func shouldLookup(md metadata.MD) bool {
	return !shouldGrpcRevalidate(md, CacheTypeClient)
}

func shouldGrpcRevalidate(md metadata.MD, cacheType string) bool {
	for _, v := range md.Get(GrpcCacheControlHeader(cacheType)) {
		if v == GrpcNoCache {
			return false
		}
		if v == GrpcMustRevalidateCache {
			return true
		}
	}
	return false
}

func requestId(v, requestId string) string {
	return fmt.Sprintf("%s_%s", v, requestId)
}

func setCacheRequestMd(md metadata.MD, fullMethod string) {
	vals := md.Get(GrpcCacheControlHeader(CacheTypeClient))
	for i, v := range vals {
		if strings.Contains(v, "max-age") {
			vals[i] = requestId(v, fullMethod)
		}
	}
	md.Set(GrpcCacheControlHeader(CacheTypeClient), vals...)
}

func getCacheControlDetails(md metadata.MD, cacheType, requestId string) (time.Duration, string) {
	for _, v := range md.Get(GrpcCacheControlHeader(cacheType)) {
		if strings.Contains(v, "max-age") {
			parts := strings.Split(v, "=")
			if len(parts) != 2 {
				continue
			}
			info := strings.Split(parts[1], "_")
			if len(info) != 2 {
				continue
			}
			ttl, err := strconv.Atoi(info[0])
			if err == nil && info[1] == requestId {
				return time.Duration(ttl) * time.Second, info[1]
			}
		}
	}
	return 0, ""
}

func checkRequestId(v, requestId string) bool {
	headerComponents := strings.Split(v, "_")
	if len(headerComponents) == 2 {
		return headerComponents[1] == requestId
	}
	return false
}

func shouldGrpcCache(md metadata.MD, cacheType, requestId string) bool {
	if md == nil {
		return false
	}
	shouldCache := false
	for _, v := range md.Get(GrpcCacheControlHeader(cacheType)) {
		if v == GrpcNoCache {
			return false
		}
		if strings.Contains(v, "max-age") {
			if checkRequestId(v, requestId) {
				shouldCache = true
			}
		}
	}
	return shouldCache
}

func isCacheMiss(md metadata.MD, cacheType string) bool {
	for _, v := range md.Get(GrpcCacheStatusHeader(cacheType)) {
		if v == GrpcCacheMiss {
			return true
		}
	}

	return false
}

func key(method string, req proto.Message) string {
	val := reflect.Indirect(reflect.ValueOf(req))
	typ := val.Type() // get the type of the underlying value
	if reflect.PtrTo(typ).Implements(reflect.TypeOf((*caching.CacheKeyer)(nil)).Elem()) {
		return req.(caching.CacheKeyer).CacheKey()
	}
	return hash(method, req)
}

type GrpcCachingInterceptor interface {
	UnaryServerInterceptor() grpc.UnaryServerInterceptor
	UnaryClientInterceptor() grpc.UnaryClientInterceptor
	storage.GrpcTtlCache
}

type GrpcClientTtlCacher struct {
	storage.GrpcTtlCache
	cacheType string
}

var _ GrpcCachingInterceptor = (*GrpcClientTtlCacher)(nil)

func NewClientGrpcTtlCacher(cache storage.GrpcTtlCache) *GrpcClientTtlCacher {
	return &GrpcClientTtlCacher{
		GrpcTtlCache: cache,
		cacheType:    CacheTypeClient,
	}
}

// if both the server and client agree on a requestId that identifies the RPC,
// continue with cache control flow log
func (g *GrpcClientTtlCacher) requestId(requestId string) metadata.MD {
	return metadata.Pairs(GrpcCacheIdentifier, requestId)
}

func (g *GrpcClientTtlCacher) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		grpc.SetHeader(ctx, cacheMiss(CacheTypeClient))
		grpc.SetHeader(ctx, g.requestId(info.FullMethod))
		return resp, err
	}
}

func (g *GrpcClientTtlCacher) key(method string, req proto.Message, ignoreCustomKeys bool) string {
	if ignoreCustomKeys {
		return hash(method, req)
	}
	return key(method, req)
}

func (g *GrpcClientTtlCacher) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lookupKey string
		var storeKey string
		clientMd, ok := metadata.FromOutgoingContext(ctx)
		// forcibly disable caching if cache-control header is set to no-cache
		if ok && noCache(clientMd, g.cacheType) {
			return invoker(ctx, method, req, reply, cc, opts...)
		} else if ok {
			setCacheRequestMd(clientMd, method)
		}

		// always try to lookup a value if we have opted-in to client-side caching
		if !ok || shouldLookup(clientMd) {
			lookupKey = g.key(method, protoimpl.X.ProtoMessageV2Of(req), ignoreCustomKeys(clientMd, g.cacheType))
			if cachedResp, ok := g.Get(lookupKey); ok {
				//set underlying data in protobuf to the cached resp
				grpc.SetHeader(ctx, cacheHit(g.cacheType))
				replyValue := reflect.ValueOf(reply).Elem()
				replyValue.Set(reflect.Indirect(reflect.ValueOf(cachedResp)))
				return nil
			}
		}
		// otherwise, let's handle the rest of this request
		var cacheStatus metadata.MD
		err := invoker(ctx, method, req, reply, cc,
			append(opts,
				grpc.Header(&cacheStatus),
			)...,
		)
		if err != nil {
			return err
		}
		if isCacheMiss(cacheStatus, g.cacheType) {
			if shouldGrpcCache(clientMd, g.cacheType, method) {
				storeKey = g.key(method, protoimpl.X.ProtoMessageV2Of(req), ignoreCustomKeys(clientMd, g.cacheType))
				ttl, _ := getCacheControlDetails(clientMd, g.cacheType, method)
				g.Set(storeKey, protoimpl.X.ProtoMessageV2Of(reply), ttl)
			}
			// server-side revalidation / forced client caching
			if shouldGrpcCache(cacheStatus, g.cacheType, method) {
				storeKey = g.key(method, protoimpl.X.ProtoMessageV2Of(req), ignoreCustomKeys(clientMd, g.cacheType))
				ttl, _ := getCacheControlDetails(cacheStatus, g.cacheType, method)
				g.Set(storeKey, protoimpl.X.ProtoMessageV2Of(reply), ttl)
			}
		}
		return nil
	}
}

func HashStrings(strings []string) string {
	var buf bytes.Buffer
	for _, s := range strings {
		buf.WriteString(fmt.Sprintf("%s-", s))
	}
	return fmt.Sprintf("%d", HashString(buf.String()))
}

func HashString(s string) uint64 {
	return xxh3.HashString(s)
}
