package cortex

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	ingesterclient "github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/hashicorp/memberlist"
	"github.com/samber/lo"
	"google.golang.org/grpc"

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
	QueryFrontend() QuerierClient
}

type DistributorClient interface {
	distributorpb.DistributorClient
	Status() (*cortexops.DistributorStatus, error)
}

type IngesterClient interface {
	ingesterclient.IngesterClient
	Status() (*cortexops.IngesterStatus, error)
}

type RulerClient interface {
	ruler.RulerClient
	Status() (*cortexops.RulerStatus, error)
}

type PurgerClient interface {
	Status() (*cortexops.PurgerStatus, error)
}

type CompactorClient interface {
	Status() (*cortexops.CompactorStatus, error)
}

type StoreGatewayClient interface {
	Status() (*cortexops.StoreGatewayStatus, error)
}

type QuerierClient interface {
	Status() (*cortexops.QuerierStatus, error)
}

type ServicesStatusClient interface {
	ServicesStatus() (*cortexops.ServiceStatusList, error)
}

type MemberlistStatusClient interface {
	MemberlistStatus() (*cortexops.MemberlistStatus, error)
}

type RingStatusClient interface {
	RingStatus() (*cortexops.ShardStatusList, error)
}

type servicesStatusClient struct {
	url        string
	httpClient *http.Client
}

func (c *servicesStatusClient) ServicesStatus() (*cortexops.ServiceStatusList, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/services", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	list := &cortexops.ServiceStatusList{}
	if err := json.NewDecoder(resp.Body).Decode(list); err != nil {
		return nil, err
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

func (c *memberlistStatusClient) MemberlistStatus() (*cortexops.MemberlistStatus, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/memberlist", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	var mr memberlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, err
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

func (c *ringStatusClient) RingStatus() (*cortexops.ShardStatusList, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/ring", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	list := &cortexops.ShardStatusList{}
	if err := json.NewDecoder(resp.Body).Decode(list); err != nil {
		return nil, err
	}
	return list, nil
}

type distributorClient struct {
	distributorpb.DistributorClient
	ServicesStatusClient
	RingStatusClient
}

func (c *distributorClient) Status() (*cortexops.DistributorStatus, error) {
	servicesStatus, err := c.ServicesStatus()
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus()
	if err != nil {
		return nil, err
	}
	return &cortexops.DistributorStatus{
		Services:     servicesStatus,
		IngesterRing: ringStatus,
	}, nil
}

type ingesterClient struct {
	ingesterclient.IngesterClient
	ServicesStatusClient
	MemberlistStatusClient
}

func (c *ingesterClient) Status() (*cortexops.IngesterStatus, error) {
	servicesStatus, err := c.ServicesStatus()
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus()
	if err != nil {
		return nil, err
	}
	return &cortexops.IngesterStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
	}, nil
}

type rulerClient struct {
	ruler.RulerClient
	ServicesStatusClient
	MemberlistStatusClient
	RingStatusClient
}

func (c *rulerClient) Status() (*cortexops.RulerStatus, error) {
	servicesStatus, err := c.ServicesStatus()
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus()
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus()
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

func (c *purgerClient) Status() (*cortexops.PurgerStatus, error) {
	servicesStatus, err := c.ServicesStatus()
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

func (c *compactorClient) Status() (*cortexops.CompactorStatus, error) {
	servicesStatus, err := c.ServicesStatus()
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus()
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus()
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

func (c *storeGatewayClient) Status() (*cortexops.StoreGatewayStatus, error) {
	servicesStatus, err := c.ServicesStatus()
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus()
	if err != nil {
		return nil, err
	}
	ringStatus, err := c.RingStatus()
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

func (c *querierClient) Status() (*cortexops.QuerierStatus, error) {
	servicesStatus, err := c.ServicesStatus()
	if err != nil {
		return nil, err
	}
	memberlistStatus, err := c.MemberlistStatus()
	if err != nil {
		return nil, err
	}
	return &cortexops.QuerierStatus{
		Services:   servicesStatus,
		Memberlist: memberlistStatus,
	}, nil
}

type clientSet struct {
	*distributorClient
	*ingesterClient
	*rulerClient
	*purgerClient
	*compactorClient
	*storeGatewayClient
	*querierClient
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

func (c *clientSet) QueryFrontend() QuerierClient {
	return c.querierClient
}

func NewClientSet(httpClient *http.Client, grpcClient *grpc.ClientConn, cortexSpec *v1beta1.CortexSpec) ClientSet {

	return &clientSet{
		distributorClient: &distributorClient{
			DistributorClient: distributorpb.NewDistributorClient(grpcClient),
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Distributor.HTTPAddress,
				httpClient: httpClient,
			},
			RingStatusClient: &ringStatusClient{
				url:        cortexSpec.Distributor.HTTPAddress,
				httpClient: httpClient,
			},
		},
		ingesterClient: &ingesterClient{
			IngesterClient: ingesterclient.NewIngesterClient(grpcClient),
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.Ingester.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.Ingester.HTTPAddress,
				httpClient: httpClient,
			},
		},
		rulerClient: &rulerClient{
			RulerClient: ruler.NewRulerClient(grpcClient),
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
				url:        cortexSpec.Compactor.HTTPAddress,
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
				url:        cortexSpec.StoreGateway.HTTPAddress,
				httpClient: httpClient,
			},
		},
		querierClient: &querierClient{
			ServicesStatusClient: &servicesStatusClient{
				url:        cortexSpec.QueryFrontend.HTTPAddress,
				httpClient: httpClient,
			},
			MemberlistStatusClient: &memberlistStatusClient{
				url:        cortexSpec.QueryFrontend.HTTPAddress,
				httpClient: httpClient,
			},
		},
	}
}
