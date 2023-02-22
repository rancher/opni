package testgrpc

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"gopkg.in/square/go-jose.v2/json"

	"github.com/rancher/opni/pkg/util"
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
