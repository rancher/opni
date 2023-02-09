package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

	GrpcCacheHit  = "hit"
	GrpcCacheMiss = "miss"
)

var (
	GrpcCacheControlHeader = func(cacheType string) string {
		return "cache-control-" + cacheType
	}
	GrpcCacheStatusHeader = func(cacheType string) string {
		return "x-cache-" + cacheType
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
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(map[string]string{})
	}
	md.Append(GrpcCacheStatusHeader(CacheTypeClient), GrpcCacheMiss)
	md.Append(GrpcCacheControlHeader(CacheTypeClient), GrpcMaxAge(ttl))
	md.Append(GrpcCacheControlHeader(CacheTypeClient), GrpcMustRevalidateCache)
	grpc.SetHeader(ctx, md)
}

func (g *GrpcClientEntityCacher) hash(method string, req proto.Message) string {
	return HashStrings([]string{method, string(Must(proto.MarshalOptions{
		Deterministic: true,
	}.Marshal(req)))})
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
		if v == GrpcMustRevalidateCache {
			return true
		}
	}
	return false
}

func shouldGrpcCache(md metadata.MD, cacheType string) bool {
	if md == nil {
		return false
	}
	header := GrpcCacheControlHeader(cacheType)
	for _, v := range md.Get(header) {
		if v == GrpcNoCache {
			return false
		}
		if strings.Contains(v, "max-age") {
			return true
		}
	}
	return false
}

func getTTL(md metadata.MD, cacheType string) time.Duration {
	for _, v := range md.Get(GrpcCacheControlHeader(cacheType)) {
		if strings.Contains(v, "max-age") {
			parts := strings.Split(v, "=")
			if len(parts) == 2 {
				ttl, err := strconv.Atoi(parts[1])
				if err == nil {
					return time.Duration(ttl) * time.Second
				}
			}
		}
	}
	return 0
}

func isCacheMiss(md metadata.MD, cacheType string) bool {
	for _, v := range md.Get(GrpcCacheStatusHeader(cacheType)) {
		if v == GrpcCacheMiss {
			return true
		}
	}

	return false
}

type GrpcCachingInterceptor interface {
	UnaryServerInterceptor() grpc.UnaryServerInterceptor
	UnaryClientInterceptor() grpc.UnaryClientInterceptor
	storage.EntityCache
}

type GrpcClientEntityCacher struct {
	storage.EntityCache
	cacheType string
}

var _ GrpcCachingInterceptor = (*GrpcClientEntityCacher)(nil)

func NewClientGrpcEntityCacher(cache storage.EntityCache) *GrpcClientEntityCacher {
	return &GrpcClientEntityCacher{
		EntityCache: cache,
		cacheType:   CacheTypeClient,
	}
}

func (g *GrpcClientEntityCacher) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		grpc.SetHeader(ctx, cacheMiss(g.cacheType))
		return resp, err
	}
}

func (g *GrpcClientEntityCacher) key(method string, req proto.Message, ignoreCustomKeys bool) string {
	if ignoreCustomKeys {
		return g.hash(method, req)
	}
	val := reflect.Indirect(reflect.ValueOf(req))
	typ := val.Type() // get the type of the underlying value
	if reflect.PtrTo(typ).Implements(reflect.TypeOf((*caching.CacheKeyer)(nil)).Elem()) {
		return req.(caching.CacheKeyer).CacheKey()
	}
	return g.hash(method, req)
}

func (g *GrpcClientEntityCacher) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lookupKey string
		var storeKey string
		clientMd, ok := metadata.FromOutgoingContext(ctx)
		// forcibly disable caching if cache-control header is set to no-cache
		if ok && noCache(clientMd, g.cacheType) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		// always try to
		if !ok || shouldLookup(clientMd) {
			lookupKey = g.key(method, req.(proto.Message), ignoreCustomKeys(clientMd, g.cacheType))
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
			if shouldGrpcCache(clientMd, g.cacheType) {
				storeKey = g.key(method, req.(proto.Message), ignoreCustomKeys(clientMd, g.cacheType))
				g.Set(storeKey, reply.(proto.Message), getTTL(clientMd, g.cacheType))
			}
			if shouldGrpcCache(cacheStatus, g.cacheType) {
				storeKey = g.key(method, req.(proto.Message), ignoreCustomKeys(clientMd, g.cacheType))
				g.Set(storeKey, reply.(proto.Message), getTTL(cacheStatus, g.cacheType))
			}
		} else {
			if shouldGrpcRevalidate(cacheStatus, g.cacheType) || shouldGrpcRevalidate(clientMd, g.cacheType) {
				storeKey = g.key(method, req.(proto.Message), ignoreCustomKeys(clientMd, g.cacheType))
				g.Set(storeKey, reply.(proto.Message), g.MaxAge())
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
