package cortex

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"

	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/hashicorp/memberlist"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/rancher/opni/pkg/config/v1beta1"
)

type HTTPClientOptions struct {
	serverNameOverride string
}

type HTTPClientOption func(*HTTPClientOptions)

func (o *HTTPClientOptions) apply(opts ...HTTPClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithServerNameOverride(serverNameOverride string) HTTPClientOption {
	return func(o *HTTPClientOptions) {
		o.serverNameOverride = serverNameOverride
	}
}

type ClientSet interface {
	Distributor() DistributorClient
	Ingester() IngesterClient
	Ruler() RulerClient
	Purger() PurgerClient
	Compactor() CompactorClient
	StoreGateway() StoreGatewayClient
	QueryFrontend() QueryFrontendClient
	Querier() QuerierClient
	HTTP(options ...HTTPClientOption) *http.Client
}

type DistributorClient interface {
	distributorpb.DistributorClient
	ConfigClient
	Status(ctx context.Context) (*cortexadmin.DistributorStatus, error)
}

type IngesterClient interface {
	Status(ctx context.Context) (*cortexadmin.IngesterStatus, error)
}

type ConfigMode string

const (
	Diff     ConfigMode = "diff"
	Defaults ConfigMode = "defaults"
)

type ConfigClient interface {
	Config(ctx context.Context, mode ...ConfigMode) (string, error)
}

type configClient struct {
	url        string
	httpClient *http.Client
}

func (c *configClient) Config(ctx context.Context, mode ...ConfigMode) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/config", c.url), nil)
	if err != nil {
		return "", err
	}
	if len(mode) > 0 {
		req.URL.RawQuery = "mode=" + string(mode[0])
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}
	return buf.String(), nil
}

type RulerClient interface {
	ruler.RulerClient
	Status(ctx context.Context) (*cortexadmin.RulerStatus, error)
}

type PurgerClient interface {
	Status(ctx context.Context) (*cortexadmin.PurgerStatus, error)
}

type CompactorClient interface {
	Status(ctx context.Context) (*cortexadmin.CompactorStatus, error)
}

type StoreGatewayClient interface {
	Status(ctx context.Context) (*cortexadmin.StoreGatewayStatus, error)
}

type QuerierClient interface {
	Status(ctx context.Context) (*cortexadmin.QuerierStatus, error)
}

type QueryFrontendClient interface {
	Status(ctx context.Context) (*cortexadmin.QueryFrontendStatus, error)
}

type ServicesStatusClient interface {
	ServicesStatus(ctx context.Context) (*cortexadmin.ServiceStatusList, error)
}

type MemberlistStatusClient interface {
	MemberlistStatus(ctx context.Context) (*cortexadmin.MemberlistStatus, error)
}

type RingStatusClient interface {
	RingStatus(ctx context.Context) (*cortexadmin.RingStatus, error)
}

type servicesStatusClient struct {
	url        string
	httpClient *http.Client
}

func (c *servicesStatusClient) ServicesStatus(ctx context.Context) (*cortexadmin.ServiceStatusList, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/services", c.url), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) {
			return nil, status.Error(codes.Internal, err.Error())
		}
		var e *net.DNSError // net.DNSError is not compatible with errors.Is
		if errors.As(err, &e) {
			return nil, status.Error(codes.Internal, err.Error()) // means configuration is unhealthy
		}
		return nil, fmt.Errorf("failed to get services status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusInternalServerError {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return nil, fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	list := &cortexadmin.ServiceStatusList{}
	if err := json.NewDecoder(resp.Body).Decode(list); err != nil {
		return nil, fmt.Errorf("failed to decode services status: %w", err)
	}
	return list, nil
}

type memberlistStatusClient struct {
	url        string
	httpClient *http.Client
}

type memberlistResponse struct {
	Members []memberlist.Node   `json:"SortedMembers"`
	Keys    map[string]struct{} `json:"Store"`
}

func (c *memberlistStatusClient) MemberlistStatus(ctx context.Context) (*cortexadmin.MemberlistStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/memberlist", c.url), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) {
			return nil, status.Error(codes.Internal, err.Error())
		}
		var e *net.DNSError // net.DNSError is not compatible with errors.Is
		if errors.As(err, &e) {
			return nil, status.Error(codes.Internal, err.Error()) //means configuration is unhealthy
		}
		return nil, fmt.Errorf("failed to get memberlist status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusInternalServerError {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return nil, fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		if buf.String() == "This instance doesn't use memberlist." {
			return &cortexadmin.MemberlistStatus{
				Enabled: false,
			}, nil
		}
		return nil, fmt.Errorf("unexpected content-type received from server: %s", contentType)
	}
	var mr memberlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("failed to decode memberlist status: %w", err)
	}
	var memberStatusItems []*cortexadmin.MemberStatus
	for _, member := range mr.Members {
		memberStatusItems = append(memberStatusItems, &cortexadmin.MemberStatus{
			Name:    member.Name,
			Address: member.Address(),
			Port:    uint32(member.Port),
			State:   int32(member.State),
		})
	}
	return &cortexadmin.MemberlistStatus{
		Keys: lo.Keys(mr.Keys),
		Members: &cortexadmin.MemberStatusList{
			Items: memberStatusItems,
		},
	}, nil
}

type ringStatusClient struct {
	url        string
	httpClient *http.Client
}

func (c *ringStatusClient) RingStatus(ctx context.Context) (*cortexadmin.RingStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/ring", c.url), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) {
			return nil, status.Error(codes.Internal, err.Error())
		}
		var e *net.DNSError // net.DNSError is not compatible with errors.Is
		if errors.As(err, &e) {
			return nil, status.Error(codes.Internal, err.Error()) // means configuration is unhealthy
		}
		return nil, fmt.Errorf("failed to get ring status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusInternalServerError {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return nil, fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		return &cortexadmin.RingStatus{
			Enabled: false,
		}, nil
	}
	list := &cortexadmin.ShardStatusList{}
	if err := json.NewDecoder(resp.Body).Decode(list); err != nil {
		return nil, fmt.Errorf("failed to decode ring status: %w", err)
	}
	return &cortexadmin.RingStatus{
		Enabled: true,
		Shards:  list,
	}, nil
}

type distributorClient struct {
	distributorpb.DistributorClient
	ConfigClient
	ServicesStatusClient
	RingStatusClient
}

func (c *distributorClient) Status(ctx context.Context) (*cortexadmin.DistributorStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.DistributorStatus{
		Services:     servicesStatus,
		IngesterRing: ringStatus,
	}, nil
}

type ingesterClient struct {
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *ingesterClient) Status(ctx context.Context) (*cortexadmin.IngesterStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus(ctx)
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.IngesterStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
		Ring:       ringStatus,
	}, nil
}

type rulerClient struct {
	ruler.RulerClient
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *rulerClient) Status(ctx context.Context) (*cortexadmin.RulerStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus(ctx)
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.RulerStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
		Ring:       ringStatus,
	}, nil
}

type purgerClient struct {
	ServicesStatusClient
}

func (c *purgerClient) Status(ctx context.Context) (*cortexadmin.PurgerStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.PurgerStatus{
		Services: servicesStatus,
	}, nil
}

type compactorClient struct {
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *compactorClient) Status(ctx context.Context) (*cortexadmin.CompactorStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus(ctx)
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.CompactorStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
		Ring:       ringStatus,
	}, nil
}

type storeGatewayClient struct {
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *storeGatewayClient) Status(ctx context.Context) (*cortexadmin.StoreGatewayStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus(ctx)
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.StoreGatewayStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
		Ring:       ringStatus,
	}, nil
}

type querierClient struct {
	ServicesStatusClient
	MemberlistStatusClient
}

func (c *querierClient) Status(ctx context.Context) (*cortexadmin.QuerierStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.QuerierStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
	}, nil
}

type queryFrontendClient struct {
	ServicesStatusClient
}

func (c *queryFrontendClient) Status(ctx context.Context) (*cortexadmin.QueryFrontendStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexadmin.QueryFrontendStatus{
		Services: servicesStatus,
	}, nil
}

type clientSet struct {
	*distributorClient
	*ingesterClient
	*rulerClient
	*purgerClient
	*compactorClient
	*storeGatewayClient
	*queryFrontendClient
	*querierClient
	*http.Client

	tlsConfig *tls.Config
}

func (c *clientSet) Distributor() DistributorClient {
	return c.distributorClient
}

func (c *clientSet) Ingester() IngesterClient {
	return c.ingesterClient
}

func (c *clientSet) Ruler() RulerClient {
	return c.rulerClient
}

func (c *clientSet) Purger() PurgerClient {
	return c.purgerClient
}

func (c *clientSet) Compactor() CompactorClient {
	return c.compactorClient
}

func (c *clientSet) StoreGateway() StoreGatewayClient {
	return c.storeGatewayClient
}

func (c *clientSet) QueryFrontend() QueryFrontendClient {
	return c.queryFrontendClient
}

func (c *clientSet) Querier() QuerierClient {
	return c.querierClient
}

func (c *clientSet) HTTP(opts ...HTTPClientOption) *http.Client {
	options := HTTPClientOptions{}
	options.apply(opts...)
	if options.serverNameOverride != "" {
		tlsConfig := c.tlsConfig.Clone()
		tlsConfig.ServerName = options.serverNameOverride
		return &http.Client{
			Transport: otelhttp.NewTransport(&http.Transport{
				TLSClientConfig: tlsConfig,
			}),
		}
	}
	return c.Client
}

func NewClientSet(ctx context.Context, cortexSpec *v1beta1.CortexSpec, tlsConfig *tls.Config) (ClientSet, error) {
	distributorCC, err := grpc.DialContext(ctx, cortexSpec.Distributor.GRPCAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, err
	}

	rulerCC, err := grpc.DialContext(ctx, cortexSpec.Ruler.GRPCAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
		grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		distributorCC.Close()
		rulerCC.Close()
	}()

	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: tlsConfig,
		}),
	}

	return &clientSet{
		distributorClient: &distributorClient{
			ConfigClient: &configClient{
				url:        cortexSpec.Distributor.HTTPAddress,
				httpClient: httpClient,
			},
			DistributorClient: distributorpb.NewDistributorClient(distributorCC),
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Distributor.HTTPAddress,
				httpClient: httpClient,
			},
			RingStatusClient: &ringStatusClient{
				url:        cortexSpec.Distributor.HTTPAddress + "/distributor",
				httpClient: httpClient,
			},
		},
		ingesterClient: &ingesterClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Ingester.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.Ingester.HTTPAddress,
				httpClient: httpClient,
			},
			RingStatusClient: &ringStatusClient{
				url:        cortexSpec.Distributor.HTTPAddress + "/ingester",
				httpClient: httpClient,
			},
		},
		rulerClient: &rulerClient{
			RulerClient: ruler.NewRulerClient(rulerCC),
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Ruler.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.Ruler.HTTPAddress,
				httpClient: httpClient,
			},
			RingStatusClient: &ringStatusClient{
				url:        cortexSpec.Ruler.HTTPAddress,
				httpClient: httpClient,
			},
		},
		purgerClient: &purgerClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Purger.HTTPAddress,
				httpClient: httpClient,
			},
		},
		compactorClient: &compactorClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Compactor.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.Compactor.HTTPAddress,
				httpClient: httpClient,
			},
			RingStatusClient: &ringStatusClient{
				url:        cortexSpec.Compactor.HTTPAddress + "/compactor",
				httpClient: httpClient,
			},
		},
		storeGatewayClient: &storeGatewayClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.StoreGateway.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.StoreGateway.HTTPAddress,
				httpClient: httpClient,
			},
			RingStatusClient: &ringStatusClient{
				url:        cortexSpec.StoreGateway.HTTPAddress + "/store-gateway",
				httpClient: httpClient,
			},
		},
		queryFrontendClient: &queryFrontendClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.QueryFrontend.HTTPAddress,
				httpClient: httpClient,
			},
		},
		querierClient: &querierClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.QueryFrontend.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.Querier.HTTPAddress,
				httpClient: httpClient,
			},
		},
		Client:    httpClient,
		tlsConfig: tlsConfig,
	}, nil
}
