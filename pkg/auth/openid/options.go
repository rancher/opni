package openid

import "net/http"

type ClientOptions struct {
	client *http.Client
}

type ClientOption func(*ClientOptions)

func (o *ClientOptions) apply(opts ...ClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithHTTPClient(client *http.Client) ClientOption {
	return func(o *ClientOptions) {
		o.client = client
	}
}
