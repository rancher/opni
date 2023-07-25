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
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/server"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/cortexproject/cortex/pkg/ingester"
	"github.com/cortexproject/cortex/pkg/querier"
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
	HttpListenAddress      string
	HttpListenPort         int
	HttpListenNetwork      string
	GrpcListenAddress      string
	GrpcListenPort         int
	GrpcListenNetwork      string
	StorageDir             string
	RuntimeConfig          string
	TLSServerConfig        TLSServerConfigShape
	TLSGatewayClientConfig TLSClientConfigShape
	TLSCortexClientConfig  TLSClientConfigShape
}

type ImplementationSpecificOverridesShape = struct {
	QueryFrontendAddress string
	MemberlistJoinAddrs  []string
	AlertmanagerURL      string
}

func StorageToBucketConfig(in *storagev1.StorageSpec) bucket.Config {
	s3Spec := in.GetS3()
	gcsSpec := in.GetGcs()
	azureSpec := in.GetAzure()
	swiftSpec := in.GetSwift()

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
	applyConfigOverrides(cfg *T) bool
}

type cortexConfigOverriderFunc[T any] func(cfg *T) bool

func (f cortexConfigOverriderFunc[T]) applyConfigOverrides(cfg any) bool {
	return f(cfg.(*T))
}

func (f cortexConfigOverriderFunc[T]) inputType() reflect.Type {
	var t T
	return reflect.TypeOf(&t).Elem()
}

// Applies all overrides in order, recursively for each field.
// If the function returns false, it will not recurse into nested struct fields.
func applyCortexConfigOverrides(spec any, appliers []CortexConfigOverrider) {
	sValue := reflect.ValueOf(spec)
	if sValue.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("bug: invalid spec type for %T: must be pointer", spec))
	}

	sType := sValue.Elem().Type()
	for _, app := range appliers {
		tType := app.inputType()
		if tType == sType {
			if !app.applyConfigOverrides(sValue.Interface()) {
				return
			}
		}
	}

	if sValue.Elem().Kind() != reflect.Struct {
		return
	}

	numFields := sValue.Elem().NumField()
	for i := 0; i < numFields; i++ {
		field := sValue.Elem().Field(i)
		// skip unexported fields
		if field.CanSet() == false {
			continue
		}
		if field.Type().Kind() == reflect.Ptr {
			if !field.IsNil() {
				applyCortexConfigOverrides(field.Interface(), appliers)
			}
		} else if field.CanAddr() {
			applyCortexConfigOverrides(field.Addr().Interface(), appliers)
		} else {
			panic(fmt.Sprintf("bug: invalid arg type for %s: must be pointer or addressable", field.Type().String()))
		}
	}
}

func NewOverrider[T any](fn func(*T) bool) CortexConfigOverrider {
	return cortexConfigOverriderFunc[T](fn)
}

type CortexConfigOverrider interface {
	applyConfigOverrides(any) bool
	inputType() reflect.Type
}

func NewTargetsOverride(targets ...string) []CortexConfigOverrider {
	return []CortexConfigOverrider{
		NewOverrider(func(in *cortex.Config) bool {
			in.Target = targets
			return true
		}),
	}
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
		NewOverrider(func(in *cortex.Config) bool {
			targets := in.Target
			if len(targets) == 1 && targets[0] == "all" {
				detectedMode = aio
			} else if len(targets) > 1 {
				detectedMode = ha
			}
			return true
		}),
		NewOverrider(func(t *distributor.HATrackerConfig) bool {
			// NB: we need to keep the distributor ha tracker disabled because
			// it's not compatible with the memberlist kv store. This it just
			// for HA prometheus which isn't supported yet anyway.
			t.EnableHATracker = false
			return false // Don't recurse into the kv config
		}),
		NewOverrider(func(in *kv.Config) bool {
			if detectedMode == ha {
				in.Store = "memberlist"
			}
			return true
		}),
		NewOverrider(func(in *ring.LifecyclerConfig) bool {
			if detectedMode == ha {
				in.JoinAfter = 10 * time.Second
				in.ObservePeriod = 10 * time.Second
			}
			return true
		}),
		NewOverrider(func(in *ring.Config) bool {
			switch detectedMode {
			case aio:
				in.ReplicationFactor = 1
			case ha:
				if in.ReplicationFactor < 3 {
					in.ReplicationFactor = 3
				}
			}
			return true
		}),
		NewOverrider(func(in *ruler.Config) bool {
			if detectedMode == ha {
				in.EnableSharding = true
			}
			return true
		}),
		NewOverrider(func(in *storegateway.Config) bool {
			if detectedMode == ha {
				in.ShardingEnabled = true
			}
			return true
		}),
		NewOverrider(func(in *storegateway.RingConfig) bool {
			switch detectedMode {
			case aio:
				in.ReplicationFactor = 1
			case ha:
				if in.ReplicationFactor < 3 {
					in.ReplicationFactor = 3
				}
			}
			return true
		}),
		NewOverrider(func(in *alertmanager.RingConfig) bool {
			switch detectedMode {
			case aio:
				in.ReplicationFactor = 1
			case ha:
				if in.ReplicationFactor < 3 {
					in.ReplicationFactor = 3
				}
			}
			return true
		}),
		NewOverrider(func(in *alertmanager.MultitenantAlertmanagerConfig) bool {
			if detectedMode == ha {
				in.ShardingEnabled = true
			}
			return true
		}),
		NewOverrider(func(in *compactor.Config) bool {
			if detectedMode == ha {
				in.ShardingEnabled = true
			}
			return true
		}),
	}
}

// These are the standard overrides that are generally always required to have
// a working Cortex config. They configure network, TLS, and filesystem settings.
func NewStandardOverrides(impl StandardOverridesShape) []CortexConfigOverrider {
	return []CortexConfigOverrider{
		NewOverrider(func(t *server.Config) bool {
			t.HTTPListenAddress = impl.HttpListenAddress
			t.HTTPListenPort = impl.HttpListenPort
			t.HTTPListenNetwork = "tcp"
			t.GRPCListenAddress = impl.GrpcListenAddress
			t.GRPCListenPort = impl.GrpcListenPort
			t.GRPCListenNetwork = "tcp"
			t.MinVersion = "VersionTLS13"
			t.HTTPTLSConfig = server.TLSConfig(impl.TLSServerConfig)
			t.GRPCTLSConfig = server.TLSConfig(impl.TLSServerConfig)
			return true
		}),
		NewOverrider(func(t *grpcclient.Config) bool {
			t.TLSEnabled = true
			t.TLS = cortextls.ClientConfig(impl.TLSCortexClientConfig)
			return true
		}),
		NewOverrider(func(t *alertmanager.ClientConfig) bool {
			t.TLSEnabled = true
			t.TLS = cortextls.ClientConfig(impl.TLSCortexClientConfig)
			return true
		}),
		NewOverrider(func(t *querier.ClientConfig) bool {
			t.TLSEnabled = true
			t.TLS = cortextls.ClientConfig(impl.TLSCortexClientConfig)
			return true
		}),
		NewOverrider(func(t *ruler.NotifierConfig) bool {
			t.TLS = cortextls.ClientConfig(impl.TLSGatewayClientConfig)
			return true
		}),
		NewOverrider(func(t *querier.Config) bool {
			t.ActiveQueryTrackerDir = filepath.Join(impl.StorageDir, "active-query-tracker")
			return true
		}),
		NewOverrider(func(t *alertmanager.MultitenantAlertmanagerConfig) bool {
			t.DataDir = filepath.Join(impl.StorageDir, "alertmanager")
			return true
		}),
		NewOverrider(func(t *compactor.Config) bool {
			t.DataDir = filepath.Join(impl.StorageDir, "compactor")
			return true
		}),
		NewOverrider(func(t *tsdb.BucketStoreConfig) bool {
			t.SyncDir = filepath.Join(impl.StorageDir, "tsdb-sync")
			return true
		}),
		NewOverrider(func(t *tsdb.TSDBConfig) bool {
			t.Dir = filepath.Join(impl.StorageDir, "tsdb")
			return true
		}),
		NewOverrider(func(t *bucket.Config) bool {
			t.Backend = "filesystem"
			t.Filesystem.Directory = filepath.Join(impl.StorageDir, "bucket")
			return true
		}),
		NewOverrider(func(t *ruler.Config) bool {
			t.RulePath = filepath.Join(impl.StorageDir, "rules")
			return true
		}),
		NewOverrider(func(t *rulestore.Config) bool {
			t.Backend = "filesystem"
			t.Filesystem.Directory = filepath.Join(impl.StorageDir, "rules")
			return true
		}),
		NewOverrider(func(t *runtimeconfig.Config) bool {
			t.LoadPath = filepath.Base(impl.RuntimeConfig)
			t.StorageConfig.Backend = "filesystem"
			t.StorageConfig.Filesystem.Directory = filepath.Dir(impl.RuntimeConfig)
			return false
		}),
	}
}

// These are overrides that are generally always required, but are specific
// to the runtime environment and are logically separate from the standard
// override set.
func NewImplementationSpecificOverrides(impl ImplementationSpecificOverridesShape) []CortexConfigOverrider {
	return []CortexConfigOverrider{
		NewOverrider(func(t *worker.Config) bool {
			t.FrontendAddress = impl.QueryFrontendAddress
			return true
		}),
		NewOverrider(func(t *memberlist.KVConfig) bool {
			t.JoinMembers = impl.MemberlistJoinAddrs
			return true
		}),
		NewOverrider(func(t *ruler.Config) bool {
			t.AlertmanagerURL = impl.AlertmanagerURL
			return true
		}),
	}
}

func MergeOverrideLists(lists ...[]CortexConfigOverrider) []CortexConfigOverrider {
	var merged []CortexConfigOverrider
	for _, l := range lists {
		merged = append(merged, l...)
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

	config := cortex.Config{}
	flagext.DefaultValues(&config)
	config.AuthEnabled = true
	config.TenantFederation.Enabled = true
	config.API.PrometheusHTTPPrefix = "/prometheus"
	config.API.ResponseCompression = true
	config.Server.GPRCServerMaxConcurrentStreams = 10000 // typo in upstream
	config.Server.GRPCServerMaxSendMsgSize = 100 << 20
	config.Server.GPRCServerMaxRecvMsgSize = 100 << 20 // typo in upstream
	config.Server.LogLevel = logLevel
	config.Server.LogFormat = logFmt
	config.Storage.Engine = "blocks"
	config.BlocksStorage.TSDB.FlushBlocksOnShutdown = true
	config.BlocksStorage.Bucket = storageConfig
	config.BlocksStorage.BucketStore.BucketIndex.Enabled = true
	config.BlocksStorage.BucketStore.SyncInterval = 5 * time.Minute
	config.BlocksStorage.BucketStore.IndexCache.Backend = "inmemory"
	config.RulerStorage.Config = storageConfig
	config.MemberlistKV.JoinMembers = nil
	config.Alertmanager.EnableAPI = true
	config.Alertmanager.ExternalURL = flagext.URLValue{
		URL: util.Must(url.Parse("/api/prom/alertmanager")),
	}
	config.Alertmanager.FallbackConfigFile = "/etc/alertmanager/fallback.yaml"
	config.Alertmanager.ShardingEnabled = false
	config.Alertmanager.ShardingRing.KVStore = kvConfig
	config.Alertmanager.ShardingRing.ReplicationFactor = 1
	config.AlertmanagerStorage.Config = storageConfig
	config.Compactor.ShardingEnabled = false
	config.Compactor.ShardingRing.KVStore = kvConfig
	config.Compactor.CleanupInterval = 5 * time.Minute
	config.Distributor.PoolConfig.HealthCheckIngesters = true
	config.Distributor.DistributorRing.KVStore = kvConfig
	config.Distributor.ShardByAllLabels = true
	config.Ingester.LifecyclerConfig.NumTokens = 512
	config.Ingester.LifecyclerConfig.RingConfig.KVStore = kvConfig
	config.Ingester.LifecyclerConfig.RingConfig.ReplicationFactor = 1
	config.IngesterClient.GRPCClientConfig.MaxSendMsgSize = 100 << 20
	config.Querier.QueryStoreForLabels = true
	config.QueryRange.SplitQueriesByInterval = 24 * time.Hour
	config.QueryRange.AlignQueriesWithStep = true
	config.QueryRange.CacheResults = true
	config.QueryRange.ResultsCacheConfig.CacheConfig.EnableFifoCache = true
	config.QueryRange.ResultsCacheConfig.CacheConfig.Fifocache.Validity = 1 * time.Hour
	config.Ruler.AlertmanangerEnableV2API = true
	config.Ruler.EnableAPI = true
	config.Ruler.Ring.KVStore = kvConfig
	config.Ruler.EnableSharding = false
	config.StoreGateway.ShardingEnabled = false
	config.StoreGateway.ShardingRing.KVStore = kvConfig
	config.StoreGateway.ShardingRing.ReplicationFactor = 1
	config.LimitsConfig.MetricRelabelConfigs = []*relabel.Config{
		metrics.OpniInternalLabelFilter(),
	}
	config.Flusher.ExitAfterFlush = false

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
			return nil, nil, fmt.Errorf("failed to unmarshal compactor config: %w\n%s", err, string(compactorData))
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
		rtConfig.Multi.PrimaryStore = multi.GetPrimary()
		rtConfig.Multi.Mirroring = multi.MirrorEnabled
	}
	if il := rt.GetIngesterLimits(); il != nil {
		rtConfig.IngesterLimits = &ingester.InstanceLimits{
			MaxIngestionRate:        il.GetMaxIngestionRate(),
			MaxInMemoryTenants:      il.GetMaxTenants(),
			MaxInMemorySeries:       il.GetMaxSeries(),
			MaxInflightPushRequests: il.GetMaxInflightPushRequests(),
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
	// encoder.SetAlwaysOmitEmpty(true)
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
	// encoder.SetAlwaysOmitEmpty(true)
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
