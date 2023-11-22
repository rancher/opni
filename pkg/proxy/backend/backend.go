package backend

import (
	"context"
	"log/slog"
	"net/url"

	"net/http/httputil"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	proxyv1 "github.com/rancher/opni/pkg/apis/proxy/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Backend interface {
	RewriteProxyRequest(string, *corev1.ReferenceList) (func(*httputil.ProxyRequest), error)
}

type implBackend struct {
	backendURL   string
	pluginClient proxyv1.RegisterProxyClient
	logger       *slog.Logger
}

func NewBackend(logger *slog.Logger, client proxyv1.RegisterProxyClient) (Backend, error) {
	info, err := client.Endpoint(context.TODO(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &implBackend{
		backendURL:   info.GetBackend(),
		pluginClient: client,
		logger:       logger,
	}, nil
}

func (b *implBackend) RewriteProxyRequest(path string, roleList *corev1.ReferenceList) (func(*httputil.ProxyRequest), error) {
	proxyURL, err := url.Parse(b.backendURL)
	if err != nil {
		b.logger.Error("failed to parse backend URL")
		return nil, err
	}

	var extraHeaders *proxyv1.HeaderResponse
	if roleList != nil {
		extraHeaders, err = b.pluginClient.AuthHeaders(context.TODO(), roleList)
		if err != nil {
			b.logger.Error("failed to fetch additional headers")
			return nil, err
		}
	}

	return func(r *httputil.ProxyRequest) {
		url.JoinPath(path)
		r.SetURL(proxyURL)

		headers := r.In.Header
		for _, header := range extraHeaders.GetHeaders() {
			headers[header.GetKey()] = header.GetValues()
		}
		r.Out.Header = headers
		r.SetXForwarded()

		query := r.In.URL.Query()
		r.Out.URL.RawQuery = query.Encode()
	}, nil
}
