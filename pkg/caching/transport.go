package caching

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gregjones/httpcache"
	"github.com/rancher/opni/pkg/storage"
)

// HttpCachingTransport
//
// !! Client libraries that use a *http.Client may overwrite the transport
// so you may want to consider using this interface with
// something like https://github.com/hashicorp/go-cleanhttp
type HttpCachingTransport interface {
	http.RoundTripper
	storage.HttpTtlCache

	Use(*http.Client) error
}

// InternalHttpCachingTransport Mostly RFC7234 compliant, see https://pkg.go.dev/github.com/gregjones/httpcache
// Should be used for internal caching only, not for LB, proxying, etc.
type InternalHttpCachingTransport struct {
	*httpcache.Transport
	storage.HttpTtlCache
}

func NewInternalHttpCacheTransport(cache storage.HttpTtlCache) *InternalHttpCachingTransport {
	return &InternalHttpCachingTransport{
		Transport:    httpcache.NewTransport(cache),
		HttpTtlCache: cache,
	}
}

func (t *InternalHttpCachingTransport) Use(client *http.Client) error {
	if client.Transport != nil && client.Transport != http.DefaultTransport {
		return fmt.Errorf("http client already has a transport")
	}
	client.Transport = t.Transport
	return nil
}

// WithHttpMaxAgeCachingHeader : header in the request or response that indicates
// the transport should set the value in the cache with ttl specified by maxAge
func WithHttpMaxAgeCachingHeader(h http.Header, maxAge time.Duration) {
	h.Set("Cache-Control", fmt.Sprintf("max-age=%d", int(maxAge.Seconds())))
}

// WithHttpNoCachingHeader : header in the request or response that indicates
// the transport should skip the cache
func WithHttpNoCachingHeader(h http.Header) {
	h.Set("Cache-Control", "no-cache")
}

// WithHttpNoStoreHeader : header in the request or response that indicates
// the response should not be stored in the cache
func WithHttpNoStoreHeader(h http.Header) {
	h.Set("Cache-Control", "no-store")
}
