package configutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"time"

	compactor_gen "github.com/rancher/opni/internal/cortex/config/compactor"
	querier_gen "github.com/rancher/opni/internal/cortex/config/querier"
	runtimeconfig_gen "github.com/rancher/opni/internal/cortex/config/runtimeconfig"
	validation_gen "github.com/rancher/opni/internal/cortex/config/validation"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/samber/lo"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/alertmanager/alertstore"
	"github.com/cortexproject/cortex/pkg/api"
	"github.com/cortexproject/cortex/pkg/chunk/cache"
	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/cortex/storage"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/cortexproject/cortex/pkg/flusher"
	"github.com/cortexproject/cortex/pkg/ingester"
	"github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/cortexproject/cortex/pkg/querier"
	"github.com/cortexproject/cortex/pkg/querier/tenantfederation"
	"github.com/cortexproject/cortex/pkg/querier/tripperware/queryrange"
	"github.com/cortexproject/cortex/pkg/querier/worker"
	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/ring/kv/memberlist"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/cortexproject/cortex/pkg/ruler/rulestore"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/bucket/azure"
	"github.com/cortexproject/cortex/pkg/storage/bucket/gcs"
	bucket_http "github.com/cortexproject/cortex/pkg/storage/bucket/http"
	"github.com/cortexproject/cortex/pkg/storage/bucket/s3"
	"github.com/cortexproject/cortex/pkg/storage/bucket/swift"
	"github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/storegateway"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/grpcclient"
	"github.com/cortexproject/cortex/pkg/util/runtimeconfig"
	cortextls "github.com/cortexproject/cortex/pkg/util/tls"
	"github.com/cortexproject/cortex/pkg/util/validation"
	kyamlv3 "github.com/kralicky/yaml/v3"

	"github.com/prometheus/prometheus/model/relabel"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/util"
)

func valueOrDefault[T any](t *T) (_ T) {
	if t == nil {
		return
	}
	return *t
}

type TLSServerConfigShape = struct {
	TLSCertPath string
	TLSKeyPath  string
	ClientAuth  string
	ClientCAs   string
}

type TLSClientConfigShape = struct {
	CertPath           string
	KeyPath            string
	CAPath             string
	ServerName         string
	InsecureSkipVerify bool
}

type StandardOverridesShape = struct {
	HttpListenAddress string
	HttpListenPort    int
	HttpListenNetwork string
	GrpcListenAddress string
	GrpcListenPort    int
	GrpcListenNetwork string
	StorageDir        string
	RuntimeConfig     string
	TLSServerConfig   TLSServerConfigShape
	TLSClientConfig   TLSClientConfigShape
}

type ImplementationSpecificOverridesShape = struct {
	QueryFrontendAddress string
	MemberlistJoinAddrs  []string
	AlertmanagerURL      string
}

func StorageToBucketConfig(in *storagev1.StorageSpec) bucket.Config {
	s3Spec := valueOrDefault(in.GetS3())
	gcsSpec := valueOrDefault(in.GetGcs())
	azureSpec := valueOrDefault(in.GetAzure())
	swiftSpec := valueOrDefault(in.GetSwift())

	storageConfig := bucket.Config{
		Backend: in.GetBackend().String(),
		S3: s3.Config{
			Endpoint:   s3Spec.GetEndpoint(),
			Region:     s3Spec.GetRegion(),
			BucketName: s3Spec.GetBucketName(),
			SecretAccessKey: flagext.Secret{
				Value: s3Spec.GetSecretAccessKey(),
			},
			AccessKeyID:      s3Spec.GetAccessKeyID(),
			Insecure:         s3Spec.GetInsecure(),
			SignatureVersion: s3Spec.GetSignatureVersion(),
			SSE: s3.SSEConfig{
				Type:                 s3Spec.GetSse().GetType(),
				KMSKeyID:             s3Spec.GetSse().GetKmsKeyID(),
				KMSEncryptionContext: s3Spec.GetSse().GetKmsEncryptionContext(),
			},
			HTTP: s3.HTTPConfig{
				Config: bucketHttpConfig(s3Spec.GetHttp()),
			},
		},
		GCS: gcs.Config{
			BucketName: gcsSpec.GetBucketName(),
			ServiceAccount: flagext.Secret{
				Value: gcsSpec.GetServiceAccount(),
			},
		},
		Azure: azure.Config{
			StorageAccountName: azureSpec.GetStorageAccountName(),
			StorageAccountKey: flagext.Secret{
				Value: azureSpec.GetStorageAccountKey(),
			},
			ContainerName: azureSpec.GetContainerName(),
			Endpoint:      azureSpec.GetEndpoint(),
			MaxRetries:    int(azureSpec.GetMaxRetries()),
			Config:        bucketHttpConfig(azureSpec.GetHttp()),
		},
		Swift: swift.Config{
			AuthVersion:       int(swiftSpec.GetAuthVersion()),
			AuthURL:           swiftSpec.GetAuthURL(),
			Username:          swiftSpec.GetUsername(),
			UserDomainName:    swiftSpec.GetUserDomainName(),
			UserDomainID:      swiftSpec.GetUserDomainID(),
			UserID:            swiftSpec.GetUserID(),
			Password:          swiftSpec.GetPassword(),
			DomainID:          swiftSpec.GetDomainID(),
			DomainName:        swiftSpec.GetDomainName(),
			ProjectID:         swiftSpec.GetProjectID(),
			ProjectName:       swiftSpec.GetProjectName(),
			ProjectDomainID:   swiftSpec.GetProjectDomainID(),
			ProjectDomainName: swiftSpec.GetProjectDomainName(),
			RegionName:        swiftSpec.GetRegionName(),
			ContainerName:     swiftSpec.GetContainerName(),
			MaxRetries:        int(swiftSpec.GetMaxRetries()),
			ConnectTimeout:    swiftSpec.GetConnectTimeout().AsDuration(),
			RequestTimeout:    swiftSpec.GetRequestTimeout().AsDuration(),
		},
	}
	return storageConfig
}

type cortexConfigOverrider[T any] interface {
	applyConfigOverrides(cfg *T)
}

type cortexConfigOverriderFunc[T any] func(cfg *T)

func (f cortexConfigOverriderFunc[T]) applyConfigOverrides(cfg any) {
	f(cfg.(*T))
}

func (f cortexConfigOverriderFunc[T]) inputType() reflect.Type {
	var t T
	return reflect.TypeOf(&t).Elem()
}

func applyCortexConfigOverrides(spec any, appliers []CortexConfigOverrider) {
	sValue := reflect.ValueOf(spec)
	if sValue.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("bug: invalid spec type for %T: must be pointer", spec))
	}
	numFields := sValue.Elem().NumField()
	for i := 0; i < numFields; i++ {
		field := sValue.Elem().Field(i)
		// skip unexported fields
		if field.CanSet() == false {
			continue
		}
		if field.Type().Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct {
			if !field.IsNil() {
				applyCortexConfigOverrides(field.Interface(), appliers)
			}
		} else if field.Type().Kind() == reflect.Struct {
			applyCortexConfigOverrides(field.Addr().Interface(), appliers)
		} else {
			for _, app := range appliers {
				tType := app.inputType()
				if field.Type() == tType {
					app.applyConfigOverrides(field.Addr().Interface())
				}
			}
		}
	}

	sType := sValue.Elem().Type()
	for _, app := range appliers {
		tType := app.inputType()
		if tType == sType {
			app.applyConfigOverrides(sValue.Interface())
		}
	}
}
func NewOverrider[T any](fn func(*T)) CortexConfigOverrider {
	return cortexConfigOverriderFunc[T](fn)
}

type CortexConfigOverrider interface {
	applyConfigOverrides(any)
	inputType() reflect.Type
}

// These overrides will automatically apply additional settings when deploying
// components in separate targets, rather than using the 'all' target.
func NewAutomaticHAOverrides() []CortexConfigOverrider {
	const (
		unknown = iota
		aio
		ha
	)
	detectedMode := unknown
	return []CortexConfigOverrider{
		NewOverrider(func(in *cortex.Config) {
			targets := in.Target
			if len(targets) == 1 && targets[0] == "all" {
				detectedMode = aio
			} else if len(targets) > 1 {
				detectedMode = ha
			}
		}),
		NewOverrider(func(in *kv.Config) {
			if detectedMode == ha {
				in.Store = "memberlist"
			}
		}),
		NewOverrider(func(in *ring.LifecyclerConfig) {
			if detectedMode == ha {
				in.JoinAfter = 10 * time.Second
				in.ObservePeriod = 10 * time.Second
			}
		}),
		NewOverrider(func(in *ring.Config) {
			switch detectedMode {
			case aio:
				in.ReplicationFactor = 1
			case ha:
				if in.ReplicationFactor < 3 {
					in.ReplicationFactor = 3
				}
			}
		}),
		NewOverrider(func(in *ruler.Config) {
			if detectedMode == ha {
				in.EnableSharding = true
			}
		}),
		NewOverrider(func(in *storegateway.Config) {
			if detectedMode == ha {
				in.ShardingEnabled = true
			}
		}),
		NewOverrider(func(in *storegateway.RingConfig) {
			switch detectedMode {
			case aio:
				in.ReplicationFactor = 1
			case ha:
				if in.ReplicationFactor < 3 {
					in.ReplicationFactor = 3
				}
			}
		}),
		NewOverrider(func(in *alertmanager.RingConfig) {
			switch detectedMode {
			case aio:
				in.ReplicationFactor = 1
			case ha:
				if in.ReplicationFactor < 3 {
					in.ReplicationFactor = 3
				}
			}
		}),
		NewOverrider(func(in *alertmanager.MultitenantAlertmanagerConfig) {
			if detectedMode == ha {
				in.ShardingEnabled = true
			}
		}),
		NewOverrider(func(in *compactor.Config) {
			if detectedMode == ha {
				in.ShardingEnabled = true
			}
		}),
	}
}

// These are the standard overrides that are generally always required to have
// a working Cortex config. They configure network, TLS, and filesystem settings.
func NewStandardOverrides(impl StandardOverridesShape) []CortexConfigOverrider {
	return []CortexConfigOverrider{
		NewOverrider(func(t *server.Config) {
			t.HTTPListenAddress = impl.HttpListenAddress
			t.HTTPListenPort = impl.HttpListenPort
			t.HTTPListenNetwork = "tcp"
			t.GRPCListenAddress = impl.GrpcListenAddress
			t.GRPCListenPort = impl.GrpcListenPort
			t.GRPCListenNetwork = "tcp"
			t.MinVersion = "VersionTLS13"
			t.HTTPTLSConfig = server.TLSConfig(impl.TLSServerConfig)
			t.GRPCTLSConfig = server.TLSConfig(impl.TLSServerConfig)
		}),
		NewOverrider(func(t *grpcclient.Config) {
			t.TLSEnabled = true
			t.TLS = cortextls.ClientConfig(impl.TLSClientConfig)
		}),
		NewOverrider(func(t *alertmanager.ClientConfig) {
			t.TLSEnabled = true
			t.TLS = cortextls.ClientConfig(impl.TLSClientConfig)
		}),
		NewOverrider(func(t *querier.ClientConfig) {
			t.TLSEnabled = true
			t.TLS = cortextls.ClientConfig(impl.TLSClientConfig)
		}),
		NewOverrider(func(t *querier.Config) {
			t.ActiveQueryTrackerDir = filepath.Join(impl.StorageDir, "active-query-tracker")
		}),
		NewOverrider(func(t *alertmanager.MultitenantAlertmanagerConfig) {
			t.DataDir = filepath.Join(impl.StorageDir, "alertmanager")
		}),
		NewOverrider(func(t *compactor.Config) {
			t.DataDir = filepath.Join(impl.StorageDir, "compactor")
		}),
		NewOverrider(func(t *tsdb.BucketStoreConfig) {
			t.SyncDir = filepath.Join(impl.StorageDir, "tsdb-sync")
		}),
		NewOverrider(func(t *tsdb.TSDBConfig) {
			t.Dir = filepath.Join(impl.StorageDir, "tsdb")
		}),
		NewOverrider(func(t *bucket.Config) {
			t.Backend = "filesystem"
			t.Filesystem.Directory = filepath.Join(impl.StorageDir, "bucket")
		}),
		NewOverrider(func(t *ruler.Config) {
			t.RulePath = filepath.Join(impl.StorageDir, "rules")
		}),
		NewOverrider(func(t *rulestore.Config) {
			t.Backend = "filesystem"
			t.Filesystem.Directory = filepath.Join(impl.StorageDir, "rules")
		}),
		NewOverrider(func(t *runtimeconfig.Config) {
			t.LoadPath = filepath.Base(impl.RuntimeConfig)
			t.StorageConfig.Backend = "filesystem"
			t.StorageConfig.Filesystem.Directory = filepath.Dir(impl.RuntimeConfig)
		}),
	}
}

// These are overrides that are generally always required, but are specific
// to the runtime environment and are logically separate from the standard
// override set.
func NewImplementationSpecificOverrides(impl ImplementationSpecificOverridesShape) []CortexConfigOverrider {
	return []CortexConfigOverrider{
		NewOverrider(func(t *worker.Config) {
			t.FrontendAddress = impl.QueryFrontendAddress
		}),
		NewOverrider(func(t *memberlist.KVConfig) {
			t.JoinMembers = impl.MemberlistJoinAddrs
		}),
		NewOverrider(func(t *ruler.Config) {
			t.AlertmanagerURL = impl.AlertmanagerURL
		}),
	}
}

func MergeOverrideLists(lists ...[]CortexConfigOverrider) []CortexConfigOverrider {
	c := make(chan CortexConfigOverrider, 1)
	merged := lo.Async(func() []CortexConfigOverrider {
		return mergeOverriders(c)
	})

	for _, list := range lists {
		for _, o := range list {
			c <- o
		}
	}
	close(c)
	return (<-merged)
}

type mergedOverrider struct {
	overriders []CortexConfigOverrider
	inType     reflect.Type
}

func (m *mergedOverrider) applyConfigOverrides(in any) {
	for _, o := range m.overriders {
		o.applyConfigOverrides(in)
	}
}

func (m *mergedOverrider) inputType() reflect.Type {
	return m.inType
}

func (m *mergedOverrider) unwrapIfSingular() CortexConfigOverrider {
	if len(m.overriders) == 1 {
		return m.overriders[0]
	}
	return m
}

func mergeOverriders(overriders <-chan CortexConfigOverrider) []CortexConfigOverrider {
	var merged []CortexConfigOverrider
	mergedOverridersByType := map[reflect.Type]*mergedOverrider{}
	for o := range overriders {
		t := o.inputType()
		if existing, ok := mergedOverridersByType[t]; ok {
			existing.overriders = append(existing.overriders, o)
		} else {
			mo := &mergedOverrider{
				overriders: []CortexConfigOverrider{o},
				inType:     t,
			}
			mergedOverridersByType[t] = mo
			merged = append(merged, mo)
		}
	}
	for i, m := range merged {
		merged[i] = m.(*mergedOverrider).unwrapIfSingular()
	}
	return merged
}

type cortexspec interface {
	GetStorage() *storagev1.StorageSpec
	GetLogLevel() string
	GetLimits() *validation_gen.Limits
	GetQuerier() *querier_gen.Config
	GetCompactor() *compactor_gen.Config
	GetRuntimeConfig() *runtimeconfig_gen.RuntimeConfigValues
}

func CortexAPISpecToCortexConfig[T cortexspec](
	in T,
	overriders ...CortexConfigOverrider,
) (*cortex.Config, *cortex.RuntimeConfigValues, error) {
	kvConfig := kv.Config{
		Store: "inmemory",
	}

	logLevel := logging.Level{}
	level := in.GetLogLevel()
	if level == "" {
		level = "info"
	}
	if err := logLevel.Set(level); err != nil {
		level = "info"
	}
	logFmt := logging.Format{}
	logFmt.Set("json")

	storageConfig := StorageToBucketConfig(in.GetStorage())

	config := cortex.Config{
		AuthEnabled: true,
		TenantFederation: tenantfederation.Config{
			Enabled: true,
		},
		API: api.Config{
			PrometheusHTTPPrefix: "/prometheus",
			ResponseCompression:  true,
		},
		Server: server.Config{
			GPRCServerMaxConcurrentStreams: 10000,
			GRPCServerMaxSendMsgSize:       100 << 20,
			GPRCServerMaxRecvMsgSize:       100 << 20, // typo in upstream
			LogLevel:                       logLevel,
			LogFormat:                      logFmt,
		},
		Storage: storage.Config{
			Engine: "blocks",
		},
		BlocksStorage: tsdb.BlocksStorageConfig{
			TSDB: tsdb.TSDBConfig{
				FlushBlocksOnShutdown: true,
			},
			Bucket: storageConfig,
			BucketStore: tsdb.BucketStoreConfig{
				BucketIndex: tsdb.BucketIndexConfig{
					Enabled: true,
				},
				SyncInterval: 5 * time.Minute,
				IndexCache: tsdb.IndexCacheConfig{
					Backend: "inmemory",
				},
			},
		},
		RulerStorage: rulestore.Config{
			Config: storageConfig,
		},
		MemberlistKV: memberlist.KVConfig{
			JoinMembers: nil,
		},

		Alertmanager: alertmanager.MultitenantAlertmanagerConfig{
			EnableAPI: true,
			ExternalURL: flagext.URLValue{
				URL: util.Must(url.Parse("/api/prom/alertmanager")),
			},
			FallbackConfigFile: "/etc/alertmanager/fallback.yaml",
			ShardingEnabled:    false,
			ShardingRing: alertmanager.RingConfig{
				KVStore:           kvConfig,
				ReplicationFactor: 1,
			},
		},
		AlertmanagerStorage: alertstore.Config{
			Config: storageConfig,
		},

		Compactor: compactor.Config{
			ShardingEnabled: false,
			ShardingRing: compactor.RingConfig{
				KVStore: kvConfig,
			},
			CleanupInterval: 5 * time.Minute,
		},
		Distributor: distributor.Config{
			PoolConfig: distributor.PoolConfig{
				HealthCheckIngesters: true,
			},
			DistributorRing: distributor.RingConfig{
				KVStore: kvConfig,
			},
			ShardByAllLabels: true,
		},
		Ingester: ingester.Config{
			LifecyclerConfig: ring.LifecyclerConfig{
				NumTokens: 512,
				RingConfig: ring.Config{
					KVStore:           kvConfig,
					ReplicationFactor: 1,
				},
			},
		},
		IngesterClient: client.Config{
			GRPCClientConfig: grpcclient.Config{
				MaxSendMsgSize: 100 << 20,
			},
		},
		Querier: querier.Config{
			QueryStoreForLabels: true,
		},
		QueryRange: queryrange.Config{
			SplitQueriesByInterval: 24 * time.Hour,
			AlignQueriesWithStep:   true,
			CacheResults:           true,
			ResultsCacheConfig: queryrange.ResultsCacheConfig{
				CacheConfig: cache.Config{
					EnableFifoCache: true,
					Fifocache: cache.FifoCacheConfig{
						Validity: 1 * time.Hour,
					},
				},
			},
		},
		Ruler: ruler.Config{
			AlertmanangerEnableV2API: true,
			EnableAPI:                true,
			Ring: ruler.RingConfig{
				KVStore: kvConfig,
			},
			EnableSharding: false,
		},
		StoreGateway: storegateway.Config{
			ShardingEnabled: false,
			ShardingRing: storegateway.RingConfig{
				KVStore:           kvConfig,
				ReplicationFactor: 1,
			},
		},
		LimitsConfig: validation.Limits{
			MetricRelabelConfigs: []*relabel.Config{
				metrics.OpniInternalLabelFilter(),
			},
		},
		Flusher: flusher.Config{
			ExitAfterFlush: false,
		},
	}

	limitsData, err := protojson.MarshalOptions{
		UseProtoNames: true,
	}.Marshal(in.GetLimits())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal limits: %w", err)
	}
	if err := yaml.Unmarshal(limitsData, &config.LimitsConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal limits: %w", err)
	}

	if in.GetCompactor() != nil {
		compactorData, err := protojson.MarshalOptions{
			UseProtoNames: true,
		}.Marshal(in.GetCompactor())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal compactor config: %w", err)
		}
		if err := yaml.Unmarshal(compactorData, &config.Compactor); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal compactor config: %w", err)
		}
	}
	if in.GetQuerier() != nil {
		querierData, err := protojson.MarshalOptions{
			UseProtoNames: true,
		}.Marshal(in.GetQuerier())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal querier config: %w", err)
		}
		if err := yaml.Unmarshal(querierData, &config.Querier); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal querier config: %w", err)
		}
	}

	applyCortexConfigOverrides(&config, overriders)

	rt := in.GetRuntimeConfig()
	rtConfig := cortex.RuntimeConfigValues{
		TenantLimits: make(map[string]*validation.Limits),
	}
	if rt != nil {
		rtConfig.IngesterChunkStreaming = rt.IngesterStreamChunksWhenUsingBlocks
	}
	if multi := rt.GetMultiKvConfig(); multi != nil {
		rtConfig.Multi.PrimaryStore = multi.Primary
		rtConfig.Multi.Mirroring = multi.MirrorEnabled
	}
	if il := rt.GetIngesterLimits(); il != nil {
		rtConfig.IngesterLimits = &ingester.InstanceLimits{
			MaxIngestionRate:        il.MaxIngestionRate,
			MaxInMemoryTenants:      il.MaxTenants,
			MaxInMemorySeries:       il.MaxSeries,
			MaxInflightPushRequests: il.MaxInflightPushRequests,
		}
	}
	for tenantId, tenantLimits := range rt.GetOverrides() {
		limits := &validation.Limits{}
		data, err := protojson.MarshalOptions{
			UseProtoNames: true,
		}.Marshal(tenantLimits)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal tenant limits: %w", err)
		}
		if err := json.Unmarshal(data, limits); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal tenant limits: %w", err)
		}
		rtConfig.TenantLimits[tenantId] = limits
	}

	return &config, &rtConfig, nil
}

func MarshalCortexConfig(config *cortex.Config) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := kyamlv3.NewEncoder(buf)
	encoder.SetAlwaysOmitEmpty(true)
	encoder.OverrideMarshalerForType(reflect.TypeOf(flagext.Secret{}),
		newOverrideMarshaler(func(s flagext.Secret) (any, error) {
			return s.Value, nil
		}),
	)
	err := encoder.Encode(config)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func MarshalRuntimeConfig(config *cortex.RuntimeConfigValues) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := kyamlv3.NewEncoder(buf)
	encoder.SetAlwaysOmitEmpty(true)
	err := encoder.Encode(config)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type overrideMarshaler[T kyamlv3.Marshaler] struct {
	fn func(T) (interface{}, error)
}

func (m *overrideMarshaler[T]) MarshalYAML(v interface{}) (interface{}, error) {
	return m.fn(v.(T))
}

func newOverrideMarshaler[T kyamlv3.Marshaler](fn func(T) (interface{}, error)) *overrideMarshaler[T] {
	return &overrideMarshaler[T]{
		fn: fn,
	}
}

func bucketHttpConfig(spec *storagev1.HTTPConfig) bucket_http.Config {
	return bucket_http.Config{
		IdleConnTimeout:       spec.GetIdleConnTimeout().AsDuration(),
		ResponseHeaderTimeout: spec.GetResponseHeaderTimeout().AsDuration(),
		InsecureSkipVerify:    spec.GetInsecureSkipVerify(),
		TLSHandshakeTimeout:   spec.GetTlsHandshakeTimeout().AsDuration(),
		ExpectContinueTimeout: spec.GetExpectContinueTimeout().AsDuration(),
		MaxIdleConns:          int(spec.GetMaxIdleConns()),
		MaxIdleConnsPerHost:   int(spec.GetMaxIdleConnsPerHost()),
		MaxConnsPerHost:       int(spec.GetMaxConnsPerHost()),
	}
}
