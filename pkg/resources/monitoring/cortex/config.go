package cortex

import (
	"bytes"
	"fmt"
	"net/url"
	"reflect"
	"time"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/alertmanager/alertstore"
	"github.com/cortexproject/cortex/pkg/api"
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
	"github.com/prometheus/node_exporter/https"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func bucketHttpConfig(spec v1beta2.HTTPConfig) bucket_http.Config {
	return bucket_http.Config{
		IdleConnTimeout:       spec.IdleConnTimeout,
		ResponseHeaderTimeout: spec.ResponseHeaderTimeout,
		InsecureSkipVerify:    spec.InsecureSkipVerify,
		TLSHandshakeTimeout:   spec.TLSHandshakeTimeout,
		ExpectContinueTimeout: spec.ExpectContinueTimeout,
		MaxIdleConns:          spec.MaxIdleConns,
		MaxIdleConnsPerHost:   spec.MaxIdleConnsPerHost,
		MaxConnsPerHost:       spec.MaxConnsPerHost,
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
	if !r.mc.Spec.Cortex.Enabled {
		return resources.Absent(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cortex",
				Namespace: r.mc.Namespace,
				Labels:    cortexAppLabel,
			},
		}), nil
	}

	s3Spec := valueOrDefault(r.mc.Spec.Cortex.Storage.S3)
	gcsSpec := valueOrDefault(r.mc.Spec.Cortex.Storage.GCS)
	azureSpec := valueOrDefault(r.mc.Spec.Cortex.Storage.Azure)
	swiftSpec := valueOrDefault(r.mc.Spec.Cortex.Storage.Swift)
	filesystemSpec := valueOrDefault(r.mc.Spec.Cortex.Storage.Filesystem)

	storageConfig := bucket.Config{
		Backend: string(r.mc.Spec.Cortex.Storage.Backend),
		S3: s3.Config{
			Endpoint:   s3Spec.Endpoint,
			Region:     s3Spec.Region,
			BucketName: s3Spec.BucketName,
			SecretAccessKey: flagext.Secret{
				Value: s3Spec.SecretAccessKey,
			},
			AccessKeyID:      s3Spec.AccessKeyID,
			Insecure:         s3Spec.Insecure,
			SignatureVersion: s3Spec.SignatureVersion,
			SSE: s3.SSEConfig{
				Type:                 s3Spec.SSE.Type,
				KMSKeyID:             s3Spec.SSE.KMSKeyID,
				KMSEncryptionContext: s3Spec.SSE.KMSEncryptionContext,
			},
			HTTP: s3.HTTPConfig{
				Config: bucketHttpConfig(s3Spec.HTTP),
			},
		},
		GCS: gcs.Config{
			BucketName: gcsSpec.BucketName,
			ServiceAccount: flagext.Secret{
				Value: gcsSpec.ServiceAccount,
			},
		},
		Azure: azure.Config{
			StorageAccountName: azureSpec.StorageAccountName,
			StorageAccountKey: flagext.Secret{
				Value: azureSpec.StorageAccountKey,
			},
			ContainerName: azureSpec.ContainerName,
			Endpoint:      azureSpec.Endpoint,
			MaxRetries:    azureSpec.MaxRetries,
			Config:        bucketHttpConfig(azureSpec.HTTP),
		},
		Swift: swift.Config{
			AuthVersion:       swiftSpec.AuthVersion,
			AuthURL:           swiftSpec.AuthURL,
			Username:          swiftSpec.Username,
			UserDomainName:    swiftSpec.UserDomainName,
			UserDomainID:      swiftSpec.UserDomainID,
			UserID:            swiftSpec.UserID,
			Password:          swiftSpec.Password,
			DomainID:          swiftSpec.DomainID,
			DomainName:        swiftSpec.DomainName,
			ProjectID:         swiftSpec.ProjectID,
			ProjectName:       swiftSpec.ProjectName,
			ProjectDomainID:   swiftSpec.ProjectDomainID,
			ProjectDomainName: swiftSpec.ProjectDomainName,
			RegionName:        swiftSpec.RegionName,
			ContainerName:     swiftSpec.ContainerName,
			MaxRetries:        swiftSpec.MaxRetries,
			ConnectTimeout:    swiftSpec.ConnectTimeout,
			RequestTimeout:    swiftSpec.RequestTimeout,
		},
		Filesystem: filesystem.Config{
			Directory: filesystemSpec.Directory,
		},
	}
	logLevel := logging.Level{}
	level := r.mc.Spec.Cortex.LogLevel
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
	tlsServerConfig := https.TLSStruct{
		TLSCertPath: "/run/cortex/certs/server/tls.crt",
		TLSKeyPath:  "/run/cortex/certs/server/tls.key",
		ClientAuth:  "RequireAndVerifyClientCert",
		ClientCAs:   "/run/cortex/certs/client/ca.crt",
	}

	kvConfig := kv.Config{
		Store: "memberlist",
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
		},
		BlocksStorage: tsdb.BlocksStorageConfig{
			TSDB: tsdb.TSDBConfig{
				Dir: "/data/tsdb",
			},
			Bucket: storageConfig,
			BucketStore: tsdb.BucketStoreConfig{
				BucketIndex: tsdb.BucketIndexConfig{
					Enabled: true,
				},
				SyncDir: "/data/tsdb-sync",
			},
		},
		RulerStorage: rulestore.Config{
			Config: storageConfig,
		},

		RuntimeConfig: runtimeconfig.Config{
			LoadPath: "/etc/cortex-runtime-config/runtime_config.yaml",
		},
		MemberlistKV: memberlist.KVConfig{
			JoinMembers: flagext.StringSlice{"cortex-memberlist"},
		},

		Alertmanager: alertmanager.MultitenantAlertmanagerConfig{
			AlertmanagerClient: alertmanager.ClientConfig{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
			},
			EnableAPI: true,
			ExternalURL: flagext.URLValue{
				URL: util.Must(url.Parse("/api/prom/alertmanager")),
			},
			FallbackConfigFile: "/etc/alertmanager/fallback.yaml",
			ShardingEnabled:    true,
			ShardingRing: alertmanager.RingConfig{
				KVStore: kvConfig,
			},
		},
		AlertmanagerStorage: alertstore.Config{
			Config: storageConfig,
		},

		Compactor: compactor.Config{
			ShardingEnabled: true,
			ShardingRing: compactor.RingConfig{
				KVStore: kvConfig,
			},
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
			FrontendAddress: fmt.Sprintf("cortex-query-frontend-headless.%s.svc.cluster.local:9095", r.mc.Namespace),
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
					KVStore: kvConfig,
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
		},
		QueryRange: queryrange.Config{
			AlignQueriesWithStep:   true,
			CacheResults:           true,
			SplitQueriesByInterval: 24 * time.Hour,
		},
		Ruler: ruler.Config{
			AlertmanagerURL:          fmt.Sprintf("https://cortex-alertmanager.%s.svc.cluster.local:8080/api/prom/alertmanager/", r.mc.Namespace),
			AlertmanangerEnableV2API: true,
			EnableAPI:                true,
			Ring: ruler.RingConfig{
				KVStore: kvConfig,
			},
			ClientTLSConfig: grpcclient.Config{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
			},
		},
		StoreGateway: storegateway.Config{
			ShardingEnabled: true,
			ShardingRing: storegateway.RingConfig{
				KVStore: kvConfig,
			},
		},
		PurgerConfig: purger.Config{
			Enable:     true,
			NumWorkers: 2,
		},
		LimitsConfig: validation.Limits{
			IngestionRate:         1e6,
			IngestionRateStrategy: "local",
			IngestionBurstSize:    2e6,
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
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string][]byte{
			"cortex.yaml": buf.Bytes(),
		},
	}

	ctrl.SetControllerReference(r.mc, secret, r.client.Scheme())
	return resources.Present(secret), nil
}

func (r *Reconciler) runtimeConfig() resources.Resource {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-runtime-config",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"runtime_config.yaml": "{}",
		},
	}
	ctrl.SetControllerReference(r.mc, cm, r.client.Scheme())
	return resources.CreatedIff(r.mc.Spec.Cortex.Enabled, cm)
}

func (r *Reconciler) alertmanagerFallbackConfig() resources.Resource {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alertmanager-fallback-config",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Data: map[string]string{
			"fallback.yaml": `global: {}
templates: []
route:
  receiver: default
receivers:
  - name: default
inhibit_rules: []
mute_time_intervals: []`,
		},
	}
	ctrl.SetControllerReference(r.mc, cm, r.client.Scheme())
	return resources.PresentIff(r.mc.Spec.Cortex.Enabled, cm)
}
