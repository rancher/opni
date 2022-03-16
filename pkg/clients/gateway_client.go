package clients

import (
	"context"
	"crypto/tls"

	"emperror.dev/errors"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/b2bmac"
	"github.com/rancher/opni-monitoring/pkg/ident"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/pkp"
)

type RequestBuilder interface {
	// Sets a request header
	Header(key, value string) RequestBuilder
	// Sets the request body
	Body(body []byte) RequestBuilder

	// Sends the request
	Send() (code int, body []byte, err error)
}

type GatewayHTTPClient interface {
	Get(ctx context.Context, path string) RequestBuilder
	Head(ctx context.Context, path string) RequestBuilder
	Post(ctx context.Context, path string) RequestBuilder
	Put(ctx context.Context, path string) RequestBuilder
	Patch(ctx context.Context, path string) RequestBuilder
	Delete(ctx context.Context, path string) RequestBuilder
}

func NewGatewayHTTPClient(
	address string,
	ip ident.Provider,
	kr keyring.Keyring,
) (GatewayHTTPClient, error) {
	if address[len(address)-1] == '/' {
		address = address[:len(address)-1]
	}
	id, err := ip.UniqueIdentifier(context.Background())
	if err != nil {
		return nil, err
	}
	var sharedKeys *keyring.SharedKeys
	var pkpKey *keyring.PKPKey
	kr.Try(
		func(sk *keyring.SharedKeys) {
			if sk != nil {
				err = errors.New("keyring contains multiple shared key sets")
				return
			}
			sharedKeys = sk
		},
		func(pk *keyring.PKPKey) {
			if pk != nil {
				err = errors.New("keyring contains multiple PKP key sets")
				return
			}
			pkpKey = pk
		},
	)
	if sharedKeys == nil {
		return nil, errors.New("keyring is missing shared keys")
	}
	if pkpKey == nil {
		return nil, errors.New("keyring is missing PKP key")
	}
	if err != nil {
		return nil, err
	}
	tlsConfig, err := pkp.TLSConfig(pkpKey.PinnedKeys)
	if err != nil {
		return nil, err
	}
	return &gatewayClient{
		address:    address,
		id:         id,
		sharedKeys: sharedKeys,
		tlsConfig:  tlsConfig,
	}, nil
}

type gatewayClient struct {
	address    string
	id         string
	sharedKeys *keyring.SharedKeys
	tlsConfig  *tls.Config
}

func (gc *gatewayClient) requestPath(path string) string {
	if path[0] != '/' {
		path = "/" + path
	}
	return gc.address + path
}

func (gc *gatewayClient) Get(ctx context.Context, path string) RequestBuilder {
	return &requestBuilder{
		gatewayClient: gc,
		req:           fiber.Get(gc.requestPath(path)).TLSConfig(gc.tlsConfig),
	}
}

func (gc *gatewayClient) Head(ctx context.Context, path string) RequestBuilder {
	return &requestBuilder{
		gatewayClient: gc,
		req:           fiber.Head(gc.requestPath(path)).TLSConfig(gc.tlsConfig),
	}
}

func (gc *gatewayClient) Post(ctx context.Context, path string) RequestBuilder {
	return &requestBuilder{
		gatewayClient: gc,
		req:           fiber.Post(gc.requestPath(path)).TLSConfig(gc.tlsConfig),
	}
}

func (gc *gatewayClient) Put(ctx context.Context, path string) RequestBuilder {
	return &requestBuilder{
		gatewayClient: gc,
		req:           fiber.Put(gc.requestPath(path)).TLSConfig(gc.tlsConfig),
	}
}

func (gc *gatewayClient) Patch(ctx context.Context, path string) RequestBuilder {
	return &requestBuilder{
		gatewayClient: gc,
		req:           fiber.Patch(gc.requestPath(path)).TLSConfig(gc.tlsConfig),
	}
}

func (gc *gatewayClient) Delete(ctx context.Context, path string) RequestBuilder {
	return &requestBuilder{
		gatewayClient: gc,
		req:           fiber.Delete(gc.requestPath(path)).TLSConfig(gc.tlsConfig),
	}
}

type requestBuilder struct {
	gatewayClient *gatewayClient
	req           *fiber.Agent
}

// Sets a request header
func (rb *requestBuilder) Header(key string, value string) RequestBuilder {
	rb.req.Set(key, value)
	return rb
}

// Sets the request body
func (rb *requestBuilder) Body(body []byte) RequestBuilder {
	rb.req.Body(body)
	return rb
}

// Sends the request
func (rb *requestBuilder) Send() (code int, body []byte, err error) {
	nonce, mac, err := b2bmac.New512([]byte(rb.gatewayClient.id),
		rb.req.Request().Body(), rb.gatewayClient.sharedKeys.ClientKey)
	if err != nil {
		return 0, nil, err
	}
	authHeader, err := b2bmac.EncodeAuthHeader([]byte(rb.gatewayClient.id), nonce, mac)
	if err != nil {
		return 0, nil, err
	}
	rb.req.Set("Authorization", authHeader)

	if err := rb.req.Parse(); err != nil {
		return 0, nil, err
	}

	code, body, errs := rb.req.Bytes()
	if len(errs) > 0 {
		return 0, nil, errors.Combine(errs...)
	}
	return code, body, nil
}
