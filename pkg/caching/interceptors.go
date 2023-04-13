package caching

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// headers
	CacheTypeClient = "client"
	CacheTypeServer = "server"
	// values

	GrpcNoCache = "no-cache"
	// server can also manually set this to revalidate the cache
	GrpcMustRevalidateCache = "must-revalidate"
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
	// maps proto message descriptor name to a function
	// that returns a string that is not guaranteed to be unique,
	// uniqueness is guaranteed when the partial key is appended to the full req method
	knownCacheableTypes = map[string]func(msg protoreflect.ProtoMessage) string{}
	knownTypesMu        sync.Mutex
)

func init() {
	RegisterProtoType(&emptypb.Empty{}, func(_ protoreflect.ProtoMessage) string {
		return "empty"
	})
}

func protoId(msg protoreflect.ProtoMessage) string {
	return string(msg.ProtoReflect().Descriptor().FullName())
}

// Register at init time a proto message from an external package to be used with the caching interceptor
func RegisterProtoType(msg proto.Message, f func(msg protoreflect.ProtoMessage) string) {
	knownTypesMu.Lock()
	defer knownTypesMu.Unlock()
	if knownCacheableTypes == nil {
		knownCacheableTypes = map[string]func(msg protoreflect.ProtoMessage) string{}
	}
	knownCacheableTypes[protoId(protoimpl.X.ProtoMessageV2Of(msg))] = f
}

func WithGrpcClientCaching(ctx context.Context, d time.Duration) context.Context {
	return WithCacheControlHeaders(ctx, CacheTypeClient, GrpcMaxAge(d))
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

// key returns the key generated from concatenating the method and the partial key generated from:
// - from the cache keyer method if the request implements the CacheKeyer interface
// - from a known map of registered proto messages
func key(method string, req protoreflect.ProtoMessage) string {
	if keyFunc, ok := knownCacheableTypes[protoId((req))]; ok {
		return fmt.Sprintf("%s/%s", method, keyFunc(req))
	}
	if keyProvider, ok := req.(CacheKeyer); ok {
		return fmt.Sprintf("%s/%s", method, keyProvider.CacheKey())
	}
	return ""
}

type GrpcCachingInterceptor interface {
	UnaryServerInterceptor() grpc.UnaryServerInterceptor
	UnaryClientInterceptor() grpc.UnaryClientInterceptor
	storage.GrpcTtlCache[protoreflect.ProtoMessage]
}

type GrpcClientTtlCacher struct {
	storage.GrpcTtlCache[protoreflect.ProtoMessage]
	cacheType string
}

var _ GrpcCachingInterceptor = (*GrpcClientTtlCacher)(nil)

func NewClientGrpcTtlCacher(cache storage.GrpcTtlCache[proto.Message]) *GrpcClientTtlCacher {
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

func (g *GrpcClientTtlCacher) key(method string, req proto.Message) string {
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
			lookupKey = g.key(method, protoimpl.X.ProtoMessageV2Of(req))
			if lookupKey != "" { // in this implementation no caching should occur
				if cachedResp, ok := g.Get(lookupKey); ok {
					//set underlying data in protobuf to the cached resp
					grpc.SetHeader(ctx, cacheHit(g.cacheType))
					replyValue := reflect.ValueOf(reply).Elem()
					replyValue.Set(reflect.Indirect(reflect.ValueOf(cachedResp)))
					return nil
				}
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
				storeKey = g.key(method, protoimpl.X.ProtoMessageV2Of(req))
				if storeKey == "" {
					return nil
				}
				ttl, _ := getCacheControlDetails(clientMd, g.cacheType, method)
				g.Set(storeKey, protoimpl.X.ProtoMessageV2Of(reply), ttl)
			}
			// server-side revalidation / forced client caching
			if shouldGrpcCache(cacheStatus, g.cacheType, method) {
				storeKey = g.key(method, protoimpl.X.ProtoMessageV2Of(req))
				if storeKey == "" {
					return nil
				}
				ttl, _ := getCacheControlDetails(cacheStatus, g.cacheType, method)
				g.Set(storeKey, protoimpl.X.ProtoMessageV2Of(reply), ttl)
			}
		}
		return nil
	}
}
