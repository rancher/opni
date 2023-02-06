package testgrpc

import (
	"fmt"
	"net/http"
	"time"

	"gopkg.in/square/go-jose.v2/json"

	"context"
	"sync/atomic"

	"github.com/rancher/opni/pkg/util"

	"google.golang.org/protobuf/types/known/emptypb"
)

type ValueResponse struct {
	Value int `json:"value"`
}

// --- Http server for caching tests ---
type CachingHttpServer struct {
	*http.Server
	value   *atomic.Int64
	objects map[string]*atomic.Int64
}

func NewCachingHttpServer(serverPort int) *CachingHttpServer {

	c := &CachingHttpServer{
		value:   &atomic.Int64{},
		objects: map[string]*atomic.Int64{},
	}

	mux := http.NewServeMux()
	// POST
	mux.HandleFunc("/increment", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			c.value.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		if _, ok := c.objects[id]; !ok {
			c.objects[id] = &atomic.Int64{}
		}
		c.objects[id].Add(1)
		w.WriteHeader(http.StatusOK)
	})
	// GET
	mux.HandleFunc("/value", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			w.WriteHeader(http.StatusOK)
			encoder := json.NewEncoder(w)
			encoder.Encode(ValueResponse{Value: int(c.value.Load())})
			return
		} else {
			if obj, ok := c.objects[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			} else {
				w.WriteHeader(http.StatusOK)
				encoder := json.NewEncoder(w)
				encoder.Encode(ValueResponse{Value: int(obj.Load())})
				return
			}
		}
	})

	mux.HandleFunc("/cache/value", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", 5))
		w.WriteHeader(http.StatusOK)
		encoder := json.NewEncoder(w)
		encoder.Encode(ValueResponse{Value: int(c.value.Load())})
		util.WithHttpMaxAgeCachingHeader(w.Header(), time.Second*5)
	})

	c.Server = &http.Server{
		Addr:           fmt.Sprintf("127.0.0.1:%d", serverPort),
		Handler:        mux,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	return c
}

type CachedServer struct {
	UnsafeCachedServiceServer
	cacheMaxAge time.Duration

	value *atomic.Int64
}

func (c *CachedServer) Increment(ctx context.Context, request *IncrementRequest) (*emptypb.Empty, error) {
	c.value.Store(c.value.Add(request.Value))
	return &emptypb.Empty{}, nil
}

func (c *CachedServer) GetValue(ctx context.Context, empty *emptypb.Empty) (*Value, error) {
	return &Value{
		Value: c.value.Load(),
	}, nil
}

func (c *CachedServer) GetValueWithForcedClientCaching(ctx context.Context, empty *emptypb.Empty) (*Value, error) {
	util.ForceClientCaching(ctx, c.cacheMaxAge)
	return &Value{
		Value: c.value.Load(),
	}, nil
}

func NewCachedServer(cacheMaxAge time.Duration) *CachedServer {
	return &CachedServer{
		value:       &atomic.Int64{},
		cacheMaxAge: cacheMaxAge,
	}
}

var _ CachedServiceServer = (*CachedServer)(nil)
