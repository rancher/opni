package v1beta1

import (
	"github.com/kralicky/opni-gateway/pkg/config/meta"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type GatewayConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec GatewayConfigSpec `json:"spec,omitempty"`
}

type GatewayConfigSpec struct {
	ListenAddress           string      `json:"listenAddress,omitempty"`
	ManagementListenAddress string      `json:"managementListenAddress,omitempty"`
	EnableMonitor           bool        `json:"enableMonitor,omitempty"`
	TrustedProxies          []string    `json:"trustedProxies,omitempty"`
	Cortex                  CortexSpec  `json:"services,omitempty"`
	AuthProvider            string      `json:"authProvider,omitempty"`
	Storage                 StorageSpec `json:"storage,omitempty"`
	Certs                   CertsSpec   `json:"certs,omitempty"`
}

type CortexSpec struct {
	Distributor   DistributorSpec   `json:"distributor,omitempty"`
	Ingester      IngesterSpec      `json:"ingester,omitempty"`
	Alertmanager  AlertmanagerSpec  `json:"alertmanager,omitempty"`
	Ruler         RulerSpec         `json:"ruler,omitempty"`
	QueryFrontend QueryFrontendSpec `json:"queryFrontend,omitempty"`
}

type DistributorSpec struct {
	Address string `json:"address,omitempty"`
}

type IngesterSpec struct {
	Address string `json:"address,omitempty"`
}

type AlertmanagerSpec struct {
	Address string `json:"address,omitempty"`
}

type RulerSpec struct {
	Address string `json:"address,omitempty"`
}

type QueryFrontendSpec struct {
	Address string `json:"address,omitempty"`
}

type CertsSpec struct {
	CACert      string `json:"caCert,omitempty"`
	ServingCert string `json:"servingCert,omitempty"`
	ServingKey  string `json:"servingKey,omitempty"`
}

func (s *GatewayConfigSpec) SetDefaults() {
	if s == nil {
		return
	}
	if s.ListenAddress == "" {
		s.ListenAddress = ":8080"
	}
	if s.Cortex.Distributor.Address == "" {
		s.Cortex.Distributor.Address = "http://cortex-distributor:8080"
	}
	if s.Cortex.Ingester.Address == "" {
		s.Cortex.Ingester.Address = "http://cortex-ingester:8080"
	}
	if s.Cortex.Alertmanager.Address == "" {
		s.Cortex.Alertmanager.Address = "http://cortex-alertmanager:8080"
	}
	if s.Cortex.Ruler.Address == "" {
		s.Cortex.Ruler.Address = "http://cortex-ruler:8080"
	}
	if s.Cortex.QueryFrontend.Address == "" {
		s.Cortex.QueryFrontend.Address = "http://cortex-query-frontend:8080"
	}
}

type StorageType string

const (
	StorageTypeEtcd   StorageType = "etcd"
	StorageTypeSecret StorageType = "secret"
)

type StorageSpec struct {
	Type StorageType      `json:"type,omitempty"`
	Etcd *EtcdStorageSpec `json:"etcd,omitempty"`
}

type EtcdStorageSpec struct {
	clientv3.Config `json:",inline"`
}
