package cortex

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/hashicorp/memberlist"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
)

type ClientSet interface {
	Distributor() DistributorClient
	Ingester() IngesterClient
	Ruler() RulerClient
	Purger() PurgerClient
	Compactor() CompactorClient
	StoreGateway() StoreGatewayClient
	QueryFrontend() QueryFrontendClient
	Querier() QuerierClient
	HTTP() *http.Client
}

type DistributorClient interface {
	distributorpb.DistributorClient
	ConfigClient
	Status(ctx context.Context) (*cortexops.DistributorStatus, error)
}

type IngesterClient interface {
	Status(ctx context.Context) (*cortexops.IngesterStatus, error)
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
	Status(ctx context.Context) (*cortexops.RulerStatus, error)
}

type PurgerClient interface {
	Status(ctx context.Context) (*cortexops.PurgerStatus, error)
}

type CompactorClient interface {
	Status(ctx context.Context) (*cortexops.CompactorStatus, error)
}

type StoreGatewayClient interface {
	Status(ctx context.Context) (*cortexops.StoreGatewayStatus, error)
}

type QuerierClient interface {
	Status(ctx context.Context) (*cortexops.QuerierStatus, error)
}

type QueryFrontendClient interface {
	Status(ctx context.Context) (*cortexops.QueryFrontendStatus, error)
}

type ServicesStatusClient interface {
	ServicesStatus(ctx context.Context) (*cortexops.ServiceStatusList, error)
}

type MemberlistStatusClient interface {
	MemberlistStatus(ctx context.Context) (*cortexops.MemberlistStatus, error)
}

type RingStatusClient interface {
	RingStatus(ctx context.Context) (*cortexops.RingStatus, error)
}

type servicesStatusClient struct {
	url        string
	httpClient *http.Client
}

func (c *servicesStatusClient) ServicesStatus(ctx context.Context) (*cortexops.ServiceStatusList, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/services", c.url), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get services status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	list := &cortexops.ServiceStatusList{}
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

func (c *memberlistStatusClient) MemberlistStatus(ctx context.Context) (*cortexops.MemberlistStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/memberlist", c.url), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get memberlist status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		if buf.String() == "This instance doesn't use memberlist." {
			return &cortexops.MemberlistStatus{
				Enabled: false,
			}, nil
		}
		return nil, fmt.Errorf("unexpected content-type received from server: %s", contentType)
	}
	var mr memberlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("failed to decode memberlist status: %w", err)
	}
	var memberStatusItems []*cortexops.MemberStatus
	for _, member := range mr.Members {
		memberStatusItems = append(memberStatusItems, &cortexops.MemberStatus{
			Name:    member.Name,
			Address: member.Address(),
			Port:    uint32(member.Port),
			State:   int32(member.State),
		})
	}
	return &cortexops.MemberlistStatus{
		Keys: lo.Keys(mr.Keys),
		Members: &cortexops.MemberStatusList{
			Items: memberStatusItems,
		},
	}, nil
}

type ringStatusClient struct {
	url        string
	httpClient *http.Client
}

func (c *ringStatusClient) RingStatus(ctx context.Context) (*cortexops.RingStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/ring", c.url), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get ring status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s (request: %s)", resp.Status, req.URL.String())
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		return &cortexops.RingStatus{
			Enabled: false,
		}, nil
	}
	list := &cortexops.ShardStatusList{}
	if err := json.NewDecoder(resp.Body).Decode(list); err != nil {
		return nil, fmt.Errorf("failed to decode ring status: %w", err)
	}
	return &cortexops.RingStatus{
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

func (c *distributorClient) Status(ctx context.Context) (*cortexops.DistributorStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexops.DistributorStatus{
		Services:     servicesStatus,
		IngesterRing: ringStatus,
	}, nil
}

type ingesterClient struct {
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *ingesterClient) Status(ctx context.Context) (*cortexops.IngesterStatus, error) {
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
	return &cortexops.IngesterStatus{
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

func (c *rulerClient) Status(ctx context.Context) (*cortexops.RulerStatus, error) {
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
	return &cortexops.RulerStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
		Ring:       ringStatus,
	}, nil
}

type purgerClient struct {
	ServicesStatusClient
}

func (c *purgerClient) Status(ctx context.Context) (*cortexops.PurgerStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexops.PurgerStatus{
		Services: servicesStatus,
	}, nil
}

type compactorClient struct {
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *compactorClient) Status(ctx context.Context) (*cortexops.CompactorStatus, error) {
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
	return &cortexops.CompactorStatus{
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

func (c *storeGatewayClient) Status(ctx context.Context) (*cortexops.StoreGatewayStatus, error) {
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
	return &cortexops.StoreGatewayStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
		Ring:       ringStatus,
	}, nil
}

type querierClient struct {
	ServicesStatusClient
	MemberlistStatusClient
}

func (c *querierClient) Status(ctx context.Context) (*cortexops.QuerierStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexops.QuerierStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
	}, nil
}

type queryFrontendClient struct {
	ServicesStatusClient
}

func (c *queryFrontendClient) Status(ctx context.Context) (*cortexops.QueryFrontendStatus, error) {
	servicesStatus, err := c.ServicesStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &cortexops.QueryFrontendStatus{
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

func (c *clientSet) HTTP() *http.Client {
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
		Client: httpClient,
	}, nil
}
