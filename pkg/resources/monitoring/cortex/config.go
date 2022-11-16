package cortex

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/alertmanager/alertstore"
	"github.com/cortexproject/cortex/pkg/api"
	"github.com/cortexproject/cortex/pkg/chunk/cache"
	"github.com/cortexproject/cortex/pkg/chunk/purger"
	"github.com/cortexproject/cortex/pkg/chunk/storage"
	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/cortexproject/cortex/pkg/frontend"
	v2 "github.com/cortexproject/cortex/pkg/frontend/v2"
	"github.com/cortexproject/cortex/pkg/ingester"
	"github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/cortexproject/cortex/pkg/querier"
	"github.com/cortexproject/cortex/pkg/querier/queryrange"
	"github.com/cortexproject/cortex/pkg/querier/tenantfederation"
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
	"github.com/cortexproject/cortex/pkg/util/tls"
	"github.com/cortexproject/cortex/pkg/util/validation"
	kyamlv3 "github.com/kralicky/yaml/v3"
	"github.com/prometheus/common/model"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func valueOrDefault[T any](t *T) (_ T) {
	if t == nil {
		return
	}
	return *t
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

func (r *Reconciler) config() (resources.Resource, error) {
	if !r.spec.Cortex.Enabled {
		return resources.Absent(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cortex",
				Namespace: r.namespace,
				Labels:    cortexAppLabel,
			},
		}), nil
	}

	if r.spec.Cortex.Storage == nil {
		r.logger.Warn("No cortex storage configured; using volatile EmptyDir storage. It is recommended to configure a storage backend.")
		r.spec.Cortex.Storage = &storagev1.StorageSpec{
			Backend: storagev1.Filesystem,
			Filesystem: &storagev1.FilesystemStorageSpec{
				Directory: "/data",
			},
		}
	}

	s3Spec := valueOrDefault(r.spec.Cortex.Storage.GetS3())
	gcsSpec := valueOrDefault(r.spec.Cortex.Storage.GetGcs())
	azureSpec := valueOrDefault(r.spec.Cortex.Storage.GetAzure())
	swiftSpec := valueOrDefault(r.spec.Cortex.Storage.GetSwift())

	storageConfig := bucket.Config{
		Backend: string(r.spec.Cortex.Storage.GetBackend()),
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
			Directory: "/data/bucket",
		},
	}
	logLevel := logging.Level{}
	level := r.spec.Cortex.LogLevel
	if level == "" {
		level = "info"
	}
	if err := logLevel.Set(level); err != nil {
		return nil, err
	}
	logFmt := logging.Format{}
	logFmt.Set("json")

	tlsClientConfig := tls.ClientConfig{
		CAPath:     "/run/cortex/certs/client/ca.crt",
		CertPath:   "/run/cortex/certs/client/tls.crt",
		KeyPath:    "/run/cortex/certs/client/tls.key",
		ServerName: "cortex-server",
	}
	tlsServerConfig := server.TLSConfig{
		TLSCertPath: "/run/cortex/certs/server/tls.crt",
		TLSKeyPath:  "/run/cortex/certs/server/tls.key",
		ClientAuth:  "RequireAndVerifyClientCert",
		ClientCAs:   "/run/cortex/certs/client/ca.crt",
	}

	isHA := r.spec.Cortex.DeploymentMode == corev1beta1.DeploymentModeHighlyAvailable

	kvConfig := kv.Config{
		Store: lo.Ternary(isHA, "memberlist", "inmemory"),
	}

	var retentionPeriod time.Duration
	if d := r.spec.Cortex.Storage.GetRetentionPeriod(); d != nil {
		duration := d.AsDuration()
		if duration > 0 && duration < 2*time.Hour {
			r.logger.Warn("Storage retention period is below the minimum required (2 hours); using default retention period (unlimited)")
		} else {
			retentionPeriod = duration
		}
	}

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
			HTTPListenPort:                 8080,
			GRPCListenPort:                 9095,
			GPRCServerMaxConcurrentStreams: 10000,
			GRPCServerMaxSendMsgSize:       100 << 20,
			GPRCServerMaxRecvMsgSize:       100 << 20, // typo in upstream
			GRPCTLSConfig:                  tlsServerConfig,
			HTTPTLSConfig:                  tlsServerConfig,
			LogLevel:                       logLevel,
			LogFormat:                      logFmt,
		},
		Storage: storage.Config{
			Engine: "blocks",
			IndexQueriesCacheConfig: cache.Config{
				EnableFifoCache: true,
				Fifocache: cache.FifoCacheConfig{
					Validity: 1 * time.Hour,
				},
			},
		},
		BlocksStorage: tsdb.BlocksStorageConfig{
			TSDB: tsdb.TSDBConfig{
				Dir:                   "/data/tsdb",
				FlushBlocksOnShutdown: true,
			},
			Bucket: storageConfig,
			BucketStore: tsdb.BucketStoreConfig{
				BucketIndex: tsdb.BucketIndexConfig{
					Enabled: true,
				},
				SyncDir:      "/data/tsdb-sync",
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
			LoadPath: "/etc/cortex-runtime-config/runtime_config.yaml",
		},
		MemberlistKV: memberlist.KVConfig{
			JoinMembers: lo.Ternary(isHA, flagext.StringSlice{"cortex-memberlist"}, nil),
		},

		Alertmanager: alertmanager.MultitenantAlertmanagerConfig{
			DataDir: "/data/alertmanager",
			AlertmanagerClient: alertmanager.ClientConfig{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
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
			DataDir:         "/data/compactor",
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
					TLS:        tlsClientConfig,
				},
			},
		},
		Worker: worker.Config{
			FrontendAddress: fmt.Sprintf("cortex-query-frontend-headless.%s.svc.cluster.local:9095", r.namespace),
			GRPCClientConfig: grpcclient.Config{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
			},
		},
		Ingester: ingester.Config{
			LifecyclerConfig: ring.LifecyclerConfig{
				JoinAfter:     10 * time.Second,
				NumTokens:     512,
				ObservePeriod: 10 * time.Second,
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
				TLS:            tlsClientConfig,
			},
		},
		Querier: querier.Config{
			ActiveQueryTrackerDir: "/data/active-query-tracker",
			StoreGatewayClient: querier.ClientConfig{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
			},
			AtModifierEnabled:   true,
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
				TLS:        tlsClientConfig,
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
		PurgerConfig: purger.Config{
			Enable: false,
		},
		LimitsConfig: validation.Limits{
			CompactorBlocksRetentionPeriod:     model.Duration(retentionPeriod),
			IngestionRateStrategy:              "local",
			IngestionRate:                      600000,
			IngestionBurstSize:                 1000000,
			MaxLocalSeriesPerUser:              math.MaxInt32,
			MaxLocalSeriesPerMetric:            math.MaxInt32,
			MaxLocalMetricsWithMetadataPerUser: math.MaxInt32,
			MaxLocalMetadataPerMetric:          math.MaxInt32,
			MaxGlobalSeriesPerUser:             math.MaxInt32,
			MaxGlobalSeriesPerMetric:           math.MaxInt32,
		},
	}

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

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex",
			Namespace: r.namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string][]byte{
			"cortex.yaml": buf.Bytes(),
		},
	}

	r.setOwner(secret)
	return resources.Present(secret), nil
}

func (r *Reconciler) runtimeConfig() resources.Resource {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-runtime-config",
			Namespace: r.namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"runtime_config.yaml": "{}",
		},
	}
	r.setOwner(cm)
	return resources.CreatedIff(r.spec.Cortex.Enabled, cm)
}

func (r *Reconciler) alertmanagerFallbackConfig() resources.Resource {
	cfgStr := `global: {}
templates: []
route:
	receiver: default
receivers:
	- name: default
inhibit_rules: []
mute_time_intervals: []`
	dConfig, err := shared.DefaultAlertManagerConfig(
		"opni-internal:8080",
	)
	if err == nil {
		cfgStr = dConfig.String()
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alertmanager-fallback-config",
			Namespace: r.namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"fallback.yaml": cfgStr,
		},
	}
	r.setOwner(cm)
	return resources.PresentIff(r.spec.Cortex.Enabled, cm)
}
