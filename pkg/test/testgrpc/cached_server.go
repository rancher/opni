package testgrpc

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/square/go-jose.v2/json"

	"github.com/rancher/opni/pkg/caching"
	"google.golang.org/grpc/codes"

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
		}
		if obj, ok := c.objects[id]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			w.WriteHeader(http.StatusOK)
			encoder := json.NewEncoder(w)
			encoder.Encode(ValueResponse{Value: int(obj.Load())})
			return
		}
	})

	mux.HandleFunc("/cache/value", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		caching.WithHttpMaxAgeCachingHeader(w.Header(), time.Second*5)
		encoder := json.NewEncoder(w)
		encoder.Encode(ValueResponse{Value: int(c.value.Load())})
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

type SimpleServer struct {
	UnsafeSimpleServiceServer

	cacheMaxAge time.Duration
	mu          *sync.Mutex
	value       int64
}

type ObjectServer struct {
	UnsafeObjectServiceServer

	cacheMaxAge time.Duration
	mu          *sync.Mutex
	items       map[string]int64
}

type AggregatorServer struct {
	UnsafeAggregatorServiceServer
	cacheMaxAge time.Duration

	SimpleServiceClient
	ObjectServiceClient
}

var _ SimpleServiceServer = (*SimpleServer)(nil)
var _ ObjectServiceServer = (*ObjectServer)(nil)
var _ AggregatorServiceServer = (*AggregatorServer)(nil)

func NewSimpleServer(cacheMaxAge time.Duration) *SimpleServer {
	return &SimpleServer{
		value:       0,
		cacheMaxAge: cacheMaxAge,
		mu:          &sync.Mutex{},
	}
}

func NewObjectServer(cacheMaxAge time.Duration) *ObjectServer {
	return &ObjectServer{
		cacheMaxAge: cacheMaxAge,
		items:       map[string]int64{},
		mu:          &sync.Mutex{},
	}
}

func NewAggregatorServer(
	cacheMaxAge time.Duration,
	simpleClient SimpleServiceClient,
	objectClient ObjectServiceClient,
) *AggregatorServer {
	return &AggregatorServer{
		cacheMaxAge:         cacheMaxAge,
		SimpleServiceClient: simpleClient,
		ObjectServiceClient: objectClient,
	}
}

func (c *SimpleServer) Increment(_ context.Context, request *IncrementRequest) (*emptypb.Empty, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += request.Value
	return &emptypb.Empty{}, nil
}

func (c *SimpleServer) GetValue(_ context.Context, _ *emptypb.Empty) (*Value, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &Value{
		Value: c.value,
	}, nil
}

func (c *SimpleServer) GetValueWithForcedClientCaching(ctx context.Context, _ *emptypb.Empty) (*Value, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	caching.ForceClientCaching(ctx, c.cacheMaxAge)
	return &Value{
		Value: c.value,
	}, nil
}

func (o *ObjectServer) IncrementObject(_ context.Context, request *IncrementObjectRequest) (*emptypb.Empty, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.items[request.Id.Id]; !ok {
		o.items[request.Id.Id] = 0
	}
	o.items[request.Id.Id] += request.Value
	return &emptypb.Empty{}, nil
}

func (o *ObjectServer) GetObjectValue(_ context.Context, reference *ObjectReference) (*Value, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.items[reference.Id]; !ok {
		return nil, util.StatusError(codes.NotFound)
	}
	returnVal := o.items[reference.Id]
	return &Value{Value: returnVal}, nil
}

func (o *ObjectServer) List(_ context.Context, _ *emptypb.Empty) (*ObjectList, error) {
	list := &ObjectList{
		Items: []*ObjectReference{},
	}
	for id := range o.items {
		list.Items = append(list.Items, &ObjectReference{
			Id: id,
		})
	}
	return list, nil
}

func (a *AggregatorServer) IncrementAll(ctx context.Context, req *IncrementRequest) (*emptypb.Empty, error) {
	_, err := a.Increment(ctx, &IncrementRequest{
		Value: req.Value,
	})
	if err != nil {
		return nil, err
	}
	objects, err := a.List(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	for _, id := range objects.Items {
		_, err := a.IncrementObject(ctx, &IncrementObjectRequest{
			Id:    id,
			Value: req.Value,
		})
		if err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

func (a *AggregatorServer) GetAll(ctx context.Context, _ *emptypb.Empty) (*Value, error) {
	returnValue := int64(0)
	val, err := a.GetValue(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	returnValue += val.Value
	objects, err := a.List(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	for _, id := range objects.Items {
		objectVal, err := a.GetObjectValue(ctx, id)
		if err != nil {
			return nil, err
		}
		returnValue += objectVal.Value
	}
	return &Value{
		Value: returnValue,
	}, nil
}

func (a *AggregatorServer) GetAllWithNestedCaching(ctx context.Context, _ *emptypb.Empty) (*Value, error) {
	returnValue := int64(0)
	val, err := a.GetValueWithForcedClientCaching(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	returnValue += val.Value
	objects, err := a.List(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	for _, id := range objects.Items {
		val, err := a.GetObjectValue(ctx, id)
		if err != nil {
			return nil, err
		}
		returnValue += val.Value
	}
	return &Value{
		Value: returnValue,
	}, nil
}

var _ caching.CacheKeyer = (*ObjectReference)(nil)

func (x *ObjectReference) CacheKey() string {
	return x.Id
}
