package clients

import (
	"context"
	"crypto/tls"
	"fmt"

	"emperror.dev/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/totem"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

type RequestBuilder interface {
	// Sets a request header
	Set(key, value string) RequestBuilder
	// Sets the request body
	Body(body []byte) RequestBuilder
	// Sends the request
	Do() (code int, body []byte, err error)

	// Deprecated: use Do() instead
	Send() (code int, body []byte, err error)
	// Deprecated: use Set() instead
	Header(key, value string) RequestBuilder
}

type GatewayHTTPClient interface {
	Get(ctx context.Context, path string) RequestBuilder
	Head(ctx context.Context, path string) RequestBuilder
	Post(ctx context.Context, path string) RequestBuilder
	Put(ctx context.Context, path string) RequestBuilder
	Patch(ctx context.Context, path string) RequestBuilder
	Delete(ctx context.Context, path string) RequestBuilder
}

type GatewayGRPCClient interface {
	grpc.ServiceRegistrar
	Connect(context.Context) (grpc.ClientConnInterface, future.Future[error])
}

func NewGatewayHTTPClient(
	address string,
	ip ident.Provider,
	kr keyring.Keyring,
	trustStrategy trust.Strategy,
) (GatewayHTTPClient, error) {
	if address[len(address)-1] == '/' {
		address = address[:len(address)-1]
	}
	client := &gatewayClient{
		httpAddress: address,
	}
	initClientShared(client, ip, kr, trustStrategy)
	return client, nil
}

func NewGatewayGRPCClient(
	address string,
	ip ident.Provider,
	kr keyring.Keyring,
	trustStrategy trust.Strategy,
) (GatewayGRPCClient, error) {
	client := &gatewayClient{
		grpcAddress: address,
	}
	initClientShared(client, ip, kr, trustStrategy)
	return client, nil
}

func initClientShared(
	client *gatewayClient,
	ip ident.Provider,
	kr keyring.Keyring,
	trustStrategy trust.Strategy,
) error {
	id, err := ip.UniqueIdentifier(context.Background())
	if err != nil {
		return err
	}
	var sharedKeys *keyring.SharedKeys
	kr.Try(func(sk *keyring.SharedKeys) {
		if sharedKeys != nil {
			err = errors.New("keyring contains multiple shared key sets")
			return
		}
		sharedKeys = sk
	})
	if err != nil {
		return err
	}
	if sharedKeys == nil {
		return errors.New("keyring is missing shared keys")
	}

	tlsConfig, err := trustStrategy.TLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create TLS config: %w", err)
	}

	client.id = id
	client.sharedKeys = sharedKeys
	client.tlsConfig = tlsConfig
	return nil
}

type gatewayClient struct {
	httpAddress string
	grpcAddress string
	id          string
	sharedKeys  *keyring.SharedKeys
	tlsConfig   *tls.Config

	services []util.ServicePack[any]
}

func (gc *gatewayClient) requestPath(path string) string {
	if path[0] != '/' {
		path = "/" + path
	}
	return gc.httpAddress + path
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

func (gc *gatewayClient) RegisterService(desc *grpc.ServiceDesc, impl any) {
	gc.services = append(gc.services, util.PackService(desc, impl))
}

func (gc *gatewayClient) Connect(ctx context.Context) (grpc.ClientConnInterface, future.Future[error]) {
	gcc, err := grpc.DialContext(ctx, gc.grpcAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(gc.tlsConfig)),
		grpc.WithStreamInterceptor(gc.streamClientInterceptor),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to dial gateway: %w", err))
	}

	streamClient := streamv1.NewStreamClient(gcc)
	stream, err := streamClient.Connect(ctx)
	if err != nil {
		return nil, future.Instant(fmt.Errorf("failed to connect to gateway: %w", err))
	}

	ts := totem.NewServer(stream)
	for _, sp := range gc.services {
		ts.RegisterService(sp.Unpack())
	}

	cc, errC := ts.Serve()
	f := future.New[error]()
	go func() {
		select {
		case <-ctx.Done():
			f.Set(ctx.Err())
		case err := <-errC:
			f.Set(err)
		}
	}()
	return cc, f
}

func (gc *gatewayClient) streamClientInterceptor(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	nonce, mac, err := b2mac.New512([]byte(gc.id), []byte(method), gc.sharedKeys.ClientKey)
	if err != nil {
		return nil, err
	}
	authHeader, err := b2mac.EncodeAuthHeader([]byte(gc.id), nonce, mac)
	if err != nil {
		return nil, err
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authHeader)
	return streamer(ctx, desc, cc, method, opts...)
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
func (rb *requestBuilder) Do() (code int, body []byte, err error) {
	nonce, mac, err := b2mac.New512([]byte(rb.gatewayClient.id),
		rb.req.Request().Body(), rb.gatewayClient.sharedKeys.ClientKey)
	if err != nil {
		return 0, nil, err
	}
	authHeader, err := b2mac.EncodeAuthHeader([]byte(rb.gatewayClient.id), nonce, mac)
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

// Deprecated: use Do() instead
func (rb *requestBuilder) Send() (code int, body []byte, err error) {
	return rb.Do()
}

// Deprecated: use Do() instead
func (rb *requestBuilder) Set(key string, value string) RequestBuilder {
	return rb.Header(key, value)
}
