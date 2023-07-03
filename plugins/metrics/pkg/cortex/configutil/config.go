package configutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/alertmanager/alertstore"
	"github.com/cortexproject/cortex/pkg/api"
	"github.com/cortexproject/cortex/pkg/chunk/cache"
	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/cortex/storage"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/cortexproject/cortex/pkg/frontend"
	v2 "github.com/cortexproject/cortex/pkg/frontend/v2"
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
	"github.com/cortexproject/cortex/pkg/storage/bucket/filesystem"
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
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
)

func valueOrDefault[T any](t *T) (_ T) {
	if t == nil {
		return
	}
	return *t
}

type ImplementationSpecificConfig struct {
	HttpListenPort       int
	GrpcListenPort       int
	StorageDir           string
	RuntimeConfig        string
	TLSServerConfig      server.TLSConfig
	TLSClientConfig      cortextls.ClientConfig
	QueryFrontendAddress string
}

func StorageToBucketConfig(
	in *storagev1.StorageSpec,
	impl ImplementationSpecificConfig,
) bucket.Config {
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
		Filesystem: filesystem.Config{
			Directory: filepath.Join(impl.StorageDir, "bucket"),
		},
	}
	return storageConfig
}

func CortexAPISpecToCortexConfig(
	in *corev1beta1.CortexSpec,
	impl ImplementationSpecificConfig,
	overrides ...func(*cortex.Config),
) (*cortex.Config, error) {
	isHA := in.DeploymentMode == corev1beta1.DeploymentModeHighlyAvailable
	kvConfig := kv.Config{
		Store: lo.Ternary(isHA, "memberlist", "inmemory"),
	}

	logLevel := logging.Level{}
	level := in.LogLevel
	if level == "" {
		level = "info"
	}
	if err := logLevel.Set(level); err != nil {
		level = "info"
	}
	logFmt := logging.Format{}
	logFmt.Set("json")

	storageConfig := StorageToBucketConfig(in.Storage, impl)

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
			HTTPListenPort:                 impl.HttpListenPort,
			GRPCListenPort:                 impl.GrpcListenPort,
			GPRCServerMaxConcurrentStreams: 10000,
			GRPCServerMaxSendMsgSize:       100 << 20,
			GPRCServerMaxRecvMsgSize:       100 << 20, // typo in upstream
			GRPCTLSConfig:                  impl.TLSServerConfig,
			HTTPTLSConfig:                  impl.TLSServerConfig,
			LogLevel:                       logLevel,
			LogFormat:                      logFmt,
		},
		Storage: storage.Config{
			Engine: "blocks",
		},
		BlocksStorage: tsdb.BlocksStorageConfig{
			TSDB: tsdb.TSDBConfig{
				Dir:                   filepath.Join(impl.StorageDir, "tsdb"),
				FlushBlocksOnShutdown: true,
			},
			Bucket: storageConfig,
			BucketStore: tsdb.BucketStoreConfig{
				BucketIndex: tsdb.BucketIndexConfig{
					Enabled: true,
				},
				SyncDir:      filepath.Join(impl.StorageDir, "tsdb-sync"),
				SyncInterval: 5 * time.Minute,
				IndexCache: tsdb.IndexCacheConfig{
					Backend: "inmemory",
				},
			},
		},
		RulerStorage: rulestore.Config{
			Config: storageConfig,
		},
		RuntimeConfig: runtimeconfig.Config{
			LoadPath: impl.RuntimeConfig,
		},
		MemberlistKV: memberlist.KVConfig{
			JoinMembers: lo.Ternary(isHA, flagext.StringSlice{"cortex-memberlist"}, nil),
		},

		Alertmanager: alertmanager.MultitenantAlertmanagerConfig{
			DataDir: filepath.Join(impl.StorageDir, "alertmanager"),
			AlertmanagerClient: alertmanager.ClientConfig{
				TLSEnabled: true,
				TLS:        impl.TLSClientConfig,
			},
			EnableAPI: true,
			ExternalURL: flagext.URLValue{
				URL: util.Must(url.Parse("/api/prom/alertmanager")),
			},
			FallbackConfigFile: "/etc/alertmanager/fallback.yaml",
			ShardingEnabled:    isHA,
			ShardingRing: alertmanager.RingConfig{
				KVStore:           kvConfig,
				ReplicationFactor: lo.Ternary(isHA, 3, 1),
			},
		},
		AlertmanagerStorage: alertstore.Config{
			Config: storageConfig,
		},

		Compactor: compactor.Config{
			ShardingEnabled: isHA,
			ShardingRing: compactor.RingConfig{
				KVStore: kvConfig,
			},
			CleanupInterval: 5 * time.Minute,
			DataDir:         filepath.Join(impl.StorageDir, "compactor"),
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
		Frontend: frontend.CombinedFrontendConfig{
			FrontendV2: v2.Config{
				GRPCClientConfig: grpcclient.Config{
					TLSEnabled: true,
					TLS:        impl.TLSClientConfig,
				},
			},
		},
		Worker: worker.Config{
			FrontendAddress: impl.QueryFrontendAddress,
			GRPCClientConfig: grpcclient.Config{
				TLSEnabled: true,
				TLS:        impl.TLSClientConfig,
			},
		},
		Ingester: ingester.Config{
			LifecyclerConfig: ring.LifecyclerConfig{
				JoinAfter:     lo.Ternary(isHA, 10*time.Second, 0),
				NumTokens:     512,
				ObservePeriod: lo.Ternary(isHA, 10*time.Second, 0),
				RingConfig: ring.Config{
					KVStore:           kvConfig,
					ReplicationFactor: lo.Ternary(isHA, 3, 1),
				},
			},
		},
		IngesterClient: client.Config{
			GRPCClientConfig: grpcclient.Config{
				MaxSendMsgSize: 100 << 20,
				TLSEnabled:     true,
				TLS:            impl.TLSClientConfig,
			},
		},
		Querier: querier.Config{
			ActiveQueryTrackerDir: filepath.Join(impl.StorageDir, "active-query-tracker"),
			StoreGatewayClient: querier.ClientConfig{
				TLSEnabled: true,
				TLS:        impl.TLSClientConfig,
			},
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
			AlertmanagerURL:          fmt.Sprintf("http://%s:9093", shared.OperatorAlertingControllerServiceName),
			AlertmanangerEnableV2API: true,
			EnableAPI:                true,
			Ring: ruler.RingConfig{
				KVStore: kvConfig,
			},
			ClientTLSConfig: grpcclient.Config{
				TLSEnabled: true,
				TLS:        impl.TLSClientConfig,
			},
			EnableSharding: isHA,
		},
		StoreGateway: storegateway.Config{
			ShardingEnabled: isHA,
			ShardingRing: storegateway.RingConfig{
				KVStore:           kvConfig,
				ReplicationFactor: lo.Ternary(isHA, 3, 1),
			},
		},
		LimitsConfig: validation.Limits{
			MetricRelabelConfigs: []*relabel.Config{
				metrics.OpniInternalLabelFilter(),
			},
		},
	}

	limitsData, err := protojson.Marshal(in.Limits)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal limits: %w", err)
	}
	if err := json.Unmarshal(limitsData, &config.LimitsConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal limits: %w", err)
	}

	if in.CompactorConfig != nil {
		compactorData, err := protojson.Marshal(in.CompactorConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal compactor config: %w", err)
		}
		if err := json.Unmarshal(compactorData, &config.Compactor); err != nil {
			return nil, fmt.Errorf("failed to unmarshal compactor config: %w", err)
		}
	}
	if in.QuerierConfig != nil {
		querierData, err := protojson.Marshal(in.QuerierConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal querier config: %w", err)
		}
		if err := json.Unmarshal(querierData, &config.Querier); err != nil {
			return nil, fmt.Errorf("failed to unmarshal querier config: %w", err)
		}
	}

	for _, overrideFn := range overrides {
		overrideFn(&config)
	}

	return &config, nil
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
