package v1beta1

import (
	"github.com/rancher/opni/pkg/config/meta"
)

type GatewayConfig struct {
	meta.TypeMeta `json:",inline"`

	Spec GatewayConfigSpec `json:"spec,omitempty"`
}

type GatewayConfigSpec struct {
	//+kubebuilder:default=":8080"
	HTTPListenAddress string `json:"httpListenAddress,omitempty"`
	//+kubebuilder:default=":9090"
	GRPCListenAddress string `json:"grpcListenAddress,omitempty"`
	//+kubebuilder:default=":8086"
	MetricsListenAddress string `json:"metricsListenAddress,omitempty"`
	//+kubebuilder:default="localhost"
	Hostname       string         `json:"hostname,omitempty"`
	Metrics        MetricsSpec    `json:"metrics,omitempty"`
	Management     ManagementSpec `json:"management,omitempty"`
	TrustedProxies []string       `json:"trustedProxies,omitempty"`
	Cortex         CortexSpec     `json:"cortex,omitempty"`
	AuthProvider   string         `json:"authProvider,omitempty"`
	Storage        StorageSpec    `json:"storage,omitempty"`
	Certs          CertsSpec      `json:"certs,omitempty"`
	Plugins        PluginsSpec    `json:"plugins,omitempty"`
	Alerting       AlertingSpec   `json:"alerting,omitempty"`
	Profiling      ProfilingSpec  `json:"profiling,omitempty"`
}

type AlertingSpec struct {
	Namespace             string `json:"Namespace,omitempty"`
	WorkerNodeService     string `json:"workerNodeService,omitempty"`
	WorkerPort            int    `json:"workerPort,omitempty"`
	WorkerStatefulSet     string `json:"workerStatefulSet,omitempty"`
	ControllerNodeService string `json:"controllerNodeService,omitempty"`
	ControllerNodePort    int    `json:"controllerNodePort,omitempty"`
	ControllerClusterPort int    `json:"controllerClusterPort,omitempty"`
	ControllerStatefulSet string `json:"controllerStatefulSet,omitempty"`
	ConfigMap             string `json:"configMap,omitempty"`
	ManagementHookHandler string `json:"managementHookHandler,omitempty"`
}

type MetricsSpec struct {
	//+kubebuilder:default="/metrics"
	Path string `json:"path,omitempty"`
}

type ProfilingSpec struct {
	//+kubebuilder:default=/debug/pprof
	Path string `json:"path,omitempty"`
}

func (s MetricsSpec) GetPath() string {
	if s.Path == "" {
		return "/metrics"
	}
	return s.Path
}

func (s ProfilingSpec) GetPath() string {
	if s.Path == "" {
		return "/debug/pprof"
	}
	return s.Path
}

type ManagementSpec struct {
	//+kubebuilder:default="tcp://0.0.0.0:11090"
	GRPCListenAddress string `json:"grpcListenAddress,omitempty"`
	//+kubebuilder:default="0.0.0.0:11080"
	HTTPListenAddress string `json:"httpListenAddress,omitempty"`
	//+kubebuilder:default="0.0.0.0:12080"
	WebListenAddress string `json:"webListenAddress,omitempty"`
}

func (m ManagementSpec) GetGRPCListenAddress() string {
	if m.GRPCListenAddress == "" {
		return "tcp://0.0.0.0:11090"
	}
	return m.GRPCListenAddress
}

func (m ManagementSpec) GetHTTPListenAddress() string {
	if m.HTTPListenAddress == "" {
		return "0.0.0.0:11080"
	}
	return m.HTTPListenAddress
}

func (m ManagementSpec) GetWebListenAddress() string {
	if m.WebListenAddress == "" {
		return "0.0.0.0:12080"
	}
	return m.WebListenAddress
}

type CortexSpec struct {
	Management    ClusterManagementSpec `json:"management,omitempty"`
	Distributor   DistributorSpec       `json:"distributor,omitempty"`
	Ingester      IngesterSpec          `json:"ingester,omitempty"`
	Alertmanager  AlertmanagerSpec      `json:"alertmanager,omitempty"`
	Compactor     CompactorSpec         `json:"compactor,omitempty"`
	StoreGateway  StoreGatewaySpec      `json:"storeGateway,omitempty"`
	Ruler         RulerSpec             `json:"ruler,omitempty"`
	QueryFrontend QueryFrontendSpec     `json:"queryFrontend,omitempty"`
	Querier       QuerierSpec           `json:"querier,omitempty"`
	Purger        PurgerSpec            `json:"purger,omitempty"`
	Certs         MTLSSpec              `json:"certs,omitempty"`
}

type ClusterManagementSpec struct {
	ClusterDriver string `json:"clusterDriver,omitempty"`
}

type DistributorSpec struct {
	//+kubebuilder:default="cortex-distributor:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
	//+kubebuilder:default="cortex-distributor-headless:9095"
	GRPCAddress string `json:"grpcAddress,omitempty"`
}

type IngesterSpec struct {
	//+kubebuilder:default="cortex-ingester:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
	//+kubebuilder:default="cortex-ingester-headless:9095"
	GRPCAddress string `json:"grpcAddress,omitempty"`
}

type AlertmanagerSpec struct {
	//+kubebuilder:default="cortex-alertmanager:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
}

type CompactorSpec struct {
	//+kubebuilder:default="cortex-compactor:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
}

type StoreGatewaySpec struct {
	//+kubebuilder:default="cortex-store-gateway:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
	//+kubebuilder:default="cortex-store-gateway-headless:9095"
	GRPCAddress string `json:"grpcAddress,omitempty"`
}

type RulerSpec struct {
	// +kubebuilder:default="cortex-ruler:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
	// +kubebuilder:default="cortex-ruler-headless:9095"
	GRPCAddress string `json:"grpcAddress,omitempty"`
}

type QueryFrontendSpec struct {
	// +kubebuilder:default="cortex-query-frontend:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
	// +kubebuilder:default="cortex-query-frontend-headless:9095"
	GRPCAddress string `json:"grpcAddress,omitempty"`
}

type QuerierSpec struct {
	// +kubebuilder:default="cortex-querier:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
}

type PurgerSpec struct {
	// +kubebuilder:default="cortex-purger:8080"
	HTTPAddress string `json:"httpAddress,omitempty"`
}

type MTLSSpec struct {
	// Path to the server CA certificate.
	ServerCA string `json:"serverCA,omitempty"`
	// Path to the client CA certificate (not needed in all cases).
	ClientCA string `json:"clientCA,omitempty"`
	// Path to the certificate used for client-cert auth.
	ClientCert string `json:"clientCert,omitempty"`
	// Path to the private key used for client-cert auth.
	ClientKey string `json:"clientKey,omitempty"`
}

type CertsSpec struct {
	// Path to a PEM encoded CA certificate file. Mutually exclusive with CACertData
	CACert *string `json:"caCert,omitempty"`
	// String containing PEM encoded CA certificate data. Mutually exclusive with CACert
	CACertData []byte `json:"caCertData,omitempty"`
	// Path to a PEM encoded server certificate file. Mutually exclusive with ServingCertData
	ServingCert *string `json:"servingCert,omitempty"`
	// String containing PEM encoded server certificate data. Mutually exclusive with ServingCert
	ServingCertData []byte `json:"servingCertData,omitempty"`
	// Path to a PEM encoded server key file. Mutually exclusive with ServingKeyData
	ServingKey *string `json:"servingKey,omitempty"`
	// String containing PEM encoded server key data. Mutually exclusive with ServingKey
	ServingKeyData []byte `json:"servingKeyData,omitempty"`
}

type PluginsSpec struct {
	// Directories to look for plugins in
	Dirs []string `json:"dirs,omitempty"`
}

func (s *GatewayConfigSpec) SetDefaults() {
	if s == nil {
		return
	}
	if s.Management.GRPCListenAddress == "" {
		s.Management.GRPCListenAddress = s.Management.GetGRPCListenAddress()
	}
	if s.Management.HTTPListenAddress == "" {
		s.Management.HTTPListenAddress = s.Management.GetHTTPListenAddress()
	}
	if s.Management.WebListenAddress == "" {
		s.Management.WebListenAddress = s.Management.GetWebListenAddress()
	}
	if s.HTTPListenAddress == "" {
		s.HTTPListenAddress = ":8080"
	}
	if s.GRPCListenAddress == "" {
		s.GRPCListenAddress = ":9090"
	}
	if s.MetricsListenAddress == "" {
		s.MetricsListenAddress = ":8086"
	}
	if s.Hostname == "" {
		s.Hostname = "localhost"
	}
	if s.Cortex.Distributor.HTTPAddress == "" {
		s.Cortex.Distributor.HTTPAddress = "cortex-distributor:8080"
	}
	if s.Cortex.Distributor.GRPCAddress == "" {
		s.Cortex.Distributor.GRPCAddress = "cortex-distributor-headless:9095"
	}
	if s.Cortex.Ingester.HTTPAddress == "" {
		s.Cortex.Ingester.HTTPAddress = "cortex-ingester:8080"
	}
	if s.Cortex.Ingester.GRPCAddress == "" {
		s.Cortex.Ingester.GRPCAddress = "cortex-ingester-headless:9095"
	}
	if s.Cortex.Alertmanager.HTTPAddress == "" {
		s.Cortex.Alertmanager.HTTPAddress = "cortex-alertmanager:8080"
	}
	if s.Cortex.Compactor.HTTPAddress == "" {
		s.Cortex.Compactor.HTTPAddress = "cortex-compactor:8080"
	}
	if s.Cortex.StoreGateway.HTTPAddress == "" {
		s.Cortex.StoreGateway.HTTPAddress = "cortex-store-gateway:8080"
	}
	if s.Cortex.StoreGateway.GRPCAddress == "" {
		s.Cortex.StoreGateway.GRPCAddress = "cortex-store-gateway-headless:9095"
	}
	if s.Cortex.Ruler.HTTPAddress == "" {
		s.Cortex.Ruler.HTTPAddress = "cortex-ruler:8080"
	}
	if s.Cortex.Ruler.GRPCAddress == "" {
		s.Cortex.Ruler.GRPCAddress = "cortex-ruler-headless:9095"
	}
	if s.Cortex.QueryFrontend.HTTPAddress == "" {
		s.Cortex.QueryFrontend.HTTPAddress = "cortex-query-frontend:8080"
	}
	if s.Cortex.QueryFrontend.GRPCAddress == "" {
		s.Cortex.QueryFrontend.GRPCAddress = "cortex-query-frontend-headless:9095"
	}
	if s.Cortex.Querier.HTTPAddress == "" {
		s.Cortex.Querier.HTTPAddress = "cortex-querier:8080"
	}
	if s.Cortex.Purger.HTTPAddress == "" {
		s.Cortex.Purger.HTTPAddress = "cortex-purger:8080"
	}
}

type StorageType string

const (
	// Use etcd for key-value storage. This is the recommended default.
	StorageTypeEtcd StorageType = "etcd"
	// Use NATS JetStream for key-value storage.
	StorageTypeJetStream StorageType = "jetstream"
	// Use Kubernetes custom resources to store objects. This is experimental,
	// and it is recommended to use the etcd storage type instead for performance
	// reasons.
	StorageTypeCRDs StorageType = "customResources"
)

type StorageSpec struct {
	Type            StorageType                 `json:"type,omitempty"`
	Etcd            *EtcdStorageSpec            `json:"etcd,omitempty"`
	JetStream       *JetStreamStorageSpec       `json:"jetstream,omitempty"`
	CustomResources *CustomResourcesStorageSpec `json:"customResources,omitempty"`
}

type EtcdStorageSpec struct {
	// List of etcd endpoints to connect to.
	Endpoints []string `json:"endpoints,omitempty"`
	// Configuration for etcd client-cert auth.
	Certs *MTLSSpec `json:"certs,omitempty"`
}

type JetStreamStorageSpec struct {
	Endpoint     string `json:"endpoint,omitempty"`
	NkeySeedPath string `json:"nkeySeedPath,omitempty"`
}

type CustomResourcesStorageSpec struct {
	// Kubernetes namespace where custom resource objects will be stored.
	Namespace string `json:"namespace,omitempty"`
}
