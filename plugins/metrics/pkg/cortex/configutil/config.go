package configutil

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"time"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/cortexproject/cortex/pkg/querier"
	"github.com/cortexproject/cortex/pkg/querier/worker"
	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/ring/kv/memberlist"
	"github.com/cortexproject/cortex/pkg/ruler"
	"github.com/cortexproject/cortex/pkg/ruler/rulestore"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/storegateway"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/grpcclient"
	"github.com/cortexproject/cortex/pkg/util/runtimeconfig"
	cortextls "github.com/cortexproject/cortex/pkg/util/tls"
	kyamlv3 "github.com/kralicky/yaml/v3"
	"github.com/prometheus/prometheus/model/relabel"
	compactor_gen "github.com/rancher/opni/internal/cortex/config/compactor"
	querier_gen "github.com/rancher/opni/internal/cortex/config/querier"
	runtimeconfig_gen "github.com/rancher/opni/internal/cortex/config/runtimeconfig"
	validation_gen "github.com/rancher/opni/internal/cortex/config/validation"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/rancher/opni/pkg/util"
	"github.com/weaveworks/common/server"
)

var cortexTargets = []string{
	"alertmanager",
	"compactor",
	"distributor",
	"ingester",
	"purger",
	"querier",
	"query-frontend",
	"ruler",
	"store-gateway",
}

func CortexTargets() []string {
	return cortexTargets
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

func newStorageOverrides(storageConfig *bucket.Config) []CortexConfigOverrider {
	return []CortexConfigOverrider{
		NewOverrider(func(in *bucket.Config) bool {
			*in = *storageConfig
			return true
		}),
	}
}

// These overrides will automatically apply additional settings when deploying
// components in separate targets, rather than using the 'all' target.
func newAutomaticHAOverrides() []CortexConfigOverrider {
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
			switch detectedMode {
			case aio:
				in.Store = "inmemory"
			case ha:
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
			switch detectedMode {
			case aio:
				in.EnableSharding = false
			case ha:
				in.EnableSharding = true
			}
			return true
		}),
		NewOverrider(func(in *storegateway.Config) bool {
			switch detectedMode {
			case aio:
				in.ShardingEnabled = false
			case ha:
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

// These are overrides that configure host options such as networking and storage.
// Generally always required to have a working Cortex config.
func NewHostOverrides(impl StandardOverridesShape) []CortexConfigOverrider {
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
	GetStorage() *storagev1.Config
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
	config := cortex.Config{}
	loadDefaults(&config)
	loadNonConfigurableStaticFields(&config)
	config.Server.LogLevel.Set(in.GetLogLevel())

	if err := LoadFromAPI(&config.LimitsConfig, in.GetLimits()); err != nil {
		return nil, nil, err
	}

	if err := LoadFromAPI(&config.Querier, in.GetQuerier()); err != nil {
		return nil, nil, err
	}

	if err := LoadFromAPI(&config.Compactor, in.GetCompactor()); err != nil {
		return nil, nil, err
	}

	storageConfig := bucket.Config{}
	loadDefaults(&storageConfig)
	if err := LoadFromAPI(&storageConfig, in.GetStorage()); err != nil {
		return nil, nil, err
	}
	applyCortexConfigOverrides(&config, newStorageOverrides(&storageConfig))
	applyCortexConfigOverrides(&config, newAutomaticHAOverrides())
	applyCortexConfigOverrides(&config, overriders)

	var rtConfig cortex.RuntimeConfigValues
	if err := LoadFromAPI(&rtConfig, in.GetRuntimeConfig()); err != nil {
		return nil, nil, err
	}

	return &config, &rtConfig, nil
}

func loadNonConfigurableStaticFields(config *cortex.Config) {
	config.AuthEnabled = true
	config.TenantFederation.Enabled = true
	config.API.PrometheusHTTPPrefix = "/prometheus"
	config.API.ResponseCompression = true
	config.Server.GPRCServerMaxConcurrentStreams = 10000 // typo in upstream
	config.Server.GRPCServerMaxSendMsgSize = 100 << 20
	config.Server.GPRCServerMaxRecvMsgSize = 100 << 20 // typo in upstream
	config.Storage.Engine = "blocks"
	config.BlocksStorage.TSDB.FlushBlocksOnShutdown = true
	config.BlocksStorage.BucketStore.BucketIndex.Enabled = true
	config.BlocksStorage.BucketStore.SyncInterval = 5 * time.Minute
	config.BlocksStorage.BucketStore.IndexCache.Backend = "inmemory"
	config.MemberlistKV.JoinMembers = nil
	config.Alertmanager.EnableAPI = true
	config.Alertmanager.ExternalURL = flagext.URLValue{
		URL: util.Must(url.Parse("/api/prom/alertmanager")),
	}
	config.Alertmanager.FallbackConfigFile = "/etc/alertmanager/fallback.yaml"
	config.Compactor.CleanupInterval = 5 * time.Minute
	config.Distributor.PoolConfig.HealthCheckIngesters = true
	config.Distributor.ShardByAllLabels = true
	config.Ingester.LifecyclerConfig.NumTokens = 512
	config.IngesterClient.GRPCClientConfig.MaxSendMsgSize = 100 << 20
	config.Querier.QueryStoreForLabels = true
	config.QueryRange.SplitQueriesByInterval = 24 * time.Hour
	config.QueryRange.AlignQueriesWithStep = true
	config.QueryRange.CacheResults = true
	config.QueryRange.ResultsCacheConfig.CacheConfig.EnableFifoCache = true
	config.QueryRange.ResultsCacheConfig.CacheConfig.Fifocache.Validity = 1 * time.Hour
	config.Ruler.AlertmanangerEnableV2API = true
	config.Ruler.EnableAPI = true
	config.LimitsConfig.MetricRelabelConfigs = []*relabel.Config{
		metrics.OpniInternalLabelFilter(),
	}
	config.Flusher.ExitAfterFlush = false
}

func MarshalCortexConfig(config *cortex.Config) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := kyamlv3.NewEncoder(buf)
	encoder.OverrideMarshalerForType(reflect.TypeOf(flagext.Secret{}),
		newOverrideMarshaler(func(s flagext.Secret) (any, error) {
			return s.Value, nil
		}),
	)
	encoder.OverrideMarshalerForType(reflect.TypeOf(flagext.StringSliceCSV{}),
		newOverrideMarshaler(func(s flagext.StringSliceCSV) (any, error) {
			// this type incorrectly interprets "" as [""], which causes problems
			if len(s) == 0 {
				return nil, nil
			}
			return s.MarshalYAML()
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
