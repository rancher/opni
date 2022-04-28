package cortex

import (
	"flag"
	"fmt"
	"net/url"
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
	"github.com/cortexproject/cortex/pkg/ring/kv/etcd"
	"github.com/cortexproject/cortex/pkg/ring/kv/memberlist"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/cortexproject/cortex/pkg/ruler/rulestore"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/storegateway"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/grpcclient"
	"github.com/cortexproject/cortex/pkg/util/log"
	"github.com/cortexproject/cortex/pkg/util/runtimeconfig"
	"github.com/cortexproject/cortex/pkg/util/tls"
	"github.com/cortexproject/cortex/pkg/util/validation"
	"github.com/imdario/mergo"
	"github.com/prometheus/common/model"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

	storageConfig, err := util.DecodeStruct[bucket.Config](r.mc.Spec.Cortex.Storage)
	if err != nil {
		return nil, err
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
	tlsServerConfig := web.TLSStruct{
		TLSCertPath: "/run/cortex/certs/server/tls.crt",
		TLSKeyPath:  "/run/cortex/certs/server/tls.key",
		ClientAuth:  "RequireAndVerifyClientCert",
		ClientCAs:   "/run/cortex/certs/client/ca.crt",
	}
	etcdKVConfig := kv.Config{
		Store: "etcd",
		StoreConfig: kv.StoreConfig{
			Etcd: etcd.Config{
				Endpoints:   []string{"etcd:2379"},
				DialTimeout: time.Minute,
				MaxRetries:  100,
				EnableTLS:   true,
				TLS: tls.ClientConfig{
					CAPath:   "/run/etcd/certs/server/ca.crt",
					CertPath: "/run/etcd/certs/client/tls.crt",
					KeyPath:  "/run/etcd/certs/client/tls.key",
				},
			},
		},
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
			GRPCServerMaxSendMsgSize:       10485760,
			GPRCServerMaxRecvMsgSize:       10485760, // typo in upstream
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
			Bucket: *storageConfig,
			BucketStore: tsdb.BucketStoreConfig{
				BucketIndex: tsdb.BucketIndexConfig{
					Enabled: true,
				},
				SyncDir: "/data/tsdb-sync",
			},
		},
		RulerStorage: rulestore.Config{
			Config: *storageConfig,
		},

		RuntimeConfig: runtimeconfig.Config{
			LoadPath: "/etc/cortex-runtime-config/runtime_config.yaml",
		},
		MemberlistKV: memberlist.KVConfig{
			TCPTransport: memberlist.TCPTransportConfig{
				BindPort: 7946,
			},
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
				KVStore: etcdKVConfig,
			},
		},
		AlertmanagerStorage: alertstore.Config{
			Config: *storageConfig,
		},

		Compactor: compactor.Config{
			ShardingEnabled: true,
			ShardingRing: compactor.RingConfig{
				KVStore: etcdKVConfig,
			},
		},
		Distributor: distributor.Config{
			HATrackerConfig: distributor.HATrackerConfig{
				EnableHATracker: false,
			},
			PoolConfig: distributor.PoolConfig{
				HealthCheckIngesters: true,
			},
			DistributorRing: distributor.RingConfig{
				KVStore: etcdKVConfig,
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
			FrontendAddress: fmt.Sprintf("-querier.frontend-address=cortex-query-frontend-headless.%s.svc.cluster.local:9095", r.mc.Namespace),
			GRPCClientConfig: grpcclient.Config{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
			},
		},
		Ingester: ingester.Config{
			LifecyclerConfig: ring.LifecyclerConfig{
				JoinAfter:     10 * time.Second,
				FinalSleep:    30 * time.Second,
				NumTokens:     512,
				ObservePeriod: 10 * time.Second,
				RingConfig: ring.Config{
					KVStore:           etcdKVConfig,
					ReplicationFactor: 1,
				},
			},
		},
		IngesterClient: client.Config{
			GRPCClientConfig: grpcclient.Config{
				MaxRecvMsgSize: 10485760,
				MaxSendMsgSize: 10485760,
				TLSEnabled:     true,
				TLS:            tlsClientConfig,
			},
		},
		LimitsConfig: validation.Limits{
			EnforceMetricName:      true,
			MaxQueryLookback:       0,
			RejectOldSamples:       true,
			RejectOldSamplesMaxAge: model.Duration(168 * time.Hour),
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
			AlertmanagerURL:          fmt.Sprintf("http://cortex-alertmanager.%s.svc.cluster.local:8080/api/prom/alertmanager/", r.mc.Namespace),
			AlertmanangerEnableV2API: true,
			EnableAPI:                true,
			Ring: ruler.RingConfig{
				KVStore: etcdKVConfig,
			},
			ClientTLSConfig: grpcclient.Config{
				TLSEnabled: true,
				TLS:        tlsClientConfig,
			},
		},
		StoreGateway: storegateway.Config{
			ShardingEnabled: true,
			ShardingRing: storegateway.RingConfig{
				KVStore: etcdKVConfig,
			},
		},
		PurgerConfig: purger.Config{
			Enable:     true,
			NumWorkers: 2,
		},
	}

	defaultConfig := cortex.Config{}
	defaultFS := flag.NewFlagSet("", flag.PanicOnError)
	defaultConfig.RegisterFlags(defaultFS)
	flagext.DefaultValues(&defaultConfig)
	util.Must(mergo.Merge(&config, defaultConfig))

	if err := config.Validate(log.Logger); err != nil {
		return nil, err
	}

	configData, err := yaml.Marshal(config)
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
			"cortex.yaml": configData,
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
