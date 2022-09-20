package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	gsync "github.com/kralicky/gpkg/sync"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type conditionStatus int32

const (
	statusPending conditionStatus = 0
	statusFailure conditionStatus = 1
)

func (s conditionStatus) String() string {
	switch s {
	case statusPending:
		return "Pending"
	case statusFailure:
		return "Failure"
	}
	return ""
}

const (
	condRemoteWrite = "Remote Write"
	condRuleSync    = "Rule Sync"
)

type Agent struct {
	controlv1.UnsafeAgentControlServer
	AgentOptions
	v1beta1.AgentConfigSpec
	router *gin.Engine
	logger *zap.SugaredLogger

	tenantID         string
	identityProvider ident.Provider
	keyringStore     storage.KeyringStore
	gatewayClient    clients.GatewayClient
	trust            trust.Strategy

	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]

	conditions gsync.Map[string, conditionStatus]
}

type AgentOptions struct {
	bootstrapper bootstrap.Bootstrapper
}

type AgentOption func(*AgentOptions)

func (o *AgentOptions) apply(opts ...AgentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithBootstrapper(bootstrapper bootstrap.Bootstrapper) AgentOption {
	return func(o *AgentOptions) {
		o.bootstrapper = bootstrapper
	}
}

func New(ctx context.Context, conf *v1beta1.AgentConfig, opts ...AgentOption) (*Agent, error) {
	options := AgentOptions{}
	options.apply(opts...)
	level := logger.DefaultLogLevel.Level()
	if conf.Spec.LogLevel != "" {
		l, err := zap.ParseAtomicLevel(conf.Spec.LogLevel)
		if err != nil {
			return nil, fmt.Errorf("error parsing log level: %w", err)
		}
		level = l.Level()
	}
	lg := logger.New(logger.WithLogLevel(level)).Named("agent")
	lg.Debugf("using log level: %s", level)

	router := gin.New()
	router.Use(logger.GinLogger(lg), gin.Recovery())
	if conf.Spec.Profiling {
		pprof.Register(router)
	}

	router.GET("/healthz", func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})

	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	ip, err := ident.GetProvider(conf.Spec.IdentityProvider)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	id, err := ip.UniqueIdentifier(initCtx)
	if err != nil {
		return nil, fmt.Errorf("error getting unique identifier: %w", err)
	}
	agent := &Agent{
		AgentOptions:     options,
		AgentConfigSpec:  conf.Spec,
		router:           router,
		logger:           lg,
		tenantID:         id,
		identityProvider: ip,
	}
	agent.initConditions()

	var keyringStoreBroker storage.KeyringStoreBroker
	switch agent.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		keyringStoreBroker = etcd.NewEtcdStore(ctx, agent.Storage.Etcd)
	case v1beta1.StorageTypeCRDs:
		keyringStoreBroker = crds.NewCRDStore()
	default:
		return nil, fmt.Errorf("unknown storage type: %s", agent.Storage.Type)
	}
	agent.keyringStore, err = keyringStoreBroker.KeyringStore("agent", &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating keyring store: %w", err)
	}

	var kr keyring.Keyring
	if options.bootstrapper != nil {
		if kr, err = agent.bootstrap(initCtx); err != nil {
			return nil, fmt.Errorf("error during bootstrap: %w", err)
		}
	} else {
		if kr, err = agent.loadKeyring(initCtx); err != nil {
			return nil, fmt.Errorf("error loading keyring: %w", err)
		}
	}

	if conf.Spec.GatewayAddress == "" {
		return nil, errors.New("gateway address not set")
	}

	agent.trust, err = agent.buildTrustStrategy(kr)
	if err != nil {
		return nil, fmt.Errorf("error building trust strategy: %w", err)
	}

	agent.gatewayClient, err = clients.NewGatewayClient(
		conf.Spec.GatewayAddress, ip, kr, agent.trust)
	if err != nil {
		return nil, fmt.Errorf("error configuring gateway client: %w", err)
	}
	controlv1.RegisterAgentControlServer(agent.gatewayClient, agent)

	var startRuleStreamOnce sync.Once
	// var startServiceDiscoveryStreamOnce sync.Once
	go func() {
		isRetry := false
		for ctx.Err() == nil {
			if isRetry {
				time.Sleep(1 * time.Second)
				lg.Info("attempting to reconnect...")
			} else {
				lg.Info("connecting to gateway...")
			}
			cc, errF := agent.gatewayClient.Connect(ctx)
			if !errF.IsSet() {
				if isRetry {
					lg.Info("gateway reconnected")
				} else {
					lg.Info("gateway connected")
				}
				agent.remoteWriteClient = clients.NewLocker(cc, remotewrite.NewRemoteWriteClient)

				startRuleStreamOnce.Do(func() {
					lg.Debug("starting rule stream")
					go func() {
						if err := agent.streamRulesToGateway(ctx); err != nil {
							lg.With(
								zap.Error(err),
							).Error("error streaming rules to gateway")
						}
					}()
				})

				// TODO : Implement
				// startServiceDiscoveryStreamOnce.Do(func() {
				// 	go agent.streamServiceDiscoveryToGateway(ctx)
				// })

				lg.With(
					zap.Error(errF.Get()), // this will block until an error is received
				).Warn("disconnected from gateway")
				agent.remoteWriteClient.Close()
			} else {
				lg.With(
					zap.Error(errF.Get()),
				).Warn("error connecting to gateway")
			}
			if util.StatusCode(errF.Get()) == codes.FailedPrecondition {
				// Non-retriable error, e.g. the cluster was deleted, or the metrics
				// capability was uninstalled.
				lg.Warn("encountered non-retriable error, exiting")
				break
			}
			isRetry = true
		}
		lg.With(
			zap.Error(ctx.Err()),
		).Warn("shutting down gateway client")
	}()

	router.POST("/api/agent/push", agent.handlePushRequest)

	return agent, nil
}

func (a *Agent) handlePushRequest(c *gin.Context) {
	var code int
	ok := a.remoteWriteClient.Use(func(rwc remotewrite.RemoteWriteClient) {
		if rwc == nil {
			a.setCondition(condRemoteWrite, statusPending, "gateway not connected")
			code = http.StatusServiceUnavailable
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Error(err)
			code = http.StatusBadRequest
			return
		}
		_, err = rwc.Push(c.Request.Context(), &remotewrite.Payload{
			AuthorizedClusterID: a.tenantID,
			Contents:            body,
		})
		if err != nil {
			stat := status.Convert(err)
			// check if statusCode is a valid HTTP status code
			if stat.Code() >= 100 && stat.Code() <= 599 {
				code = int(stat.Code())
			} else {
				code = http.StatusServiceUnavailable
			}
			// As a special case, status code 400 may indicate a success.
			// Cortex handles a variety of cases where prometheus would normally
			// return an error, such as duplicate or out of order samples. Cortex
			// will return code 400 to prometheus, which prometheus will treat as
			// a non-retriable error. In this case, the remote write status condition
			// will be cleared as if the request succeeded.
			if code == http.StatusBadRequest {
				a.clearCondition(condRemoteWrite)
				c.Error(errors.New("soft error: this request likely succeeded"))
			} else {
				a.setCondition(condRemoteWrite, statusFailure, stat.Message())
			}
			c.Error(err)
			return
		}
		a.clearCondition(condRemoteWrite)
		code = http.StatusOK
	})
	if !ok {
		code = http.StatusServiceUnavailable
	}
	c.Status(code)
}

func (a *Agent) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp4", a.AgentConfigSpec.ListenAddress)
	if err != nil {
		return err
	}
	return util.ServeHandler(ctx, a.router.Handler(), listener)
}

func (a *Agent) bootstrap(ctx context.Context) (keyring.Keyring, error) {
	lg := a.logger

	// Load the stored keyring, or bootstrap a new one if it doesn't exist
	if _, err := a.keyringStore.Get(ctx); errors.Is(err, storage.ErrNotFound) {
		lg.Info("performing initial bootstrap")
		newKeyring, err := a.bootstrapper.Bootstrap(ctx, a.identityProvider)
		if err != nil {
			return nil, err
		}
		lg.Info("bootstrap completed successfully")
		for {
			// Don't let this fail easily, otherwise we will lose the keyring forever.
			// Keep retrying until it succeeds.
			err = a.keyringStore.Put(ctx, newKeyring)
			if err != nil {
				lg.With(zap.Error(err)).Error("failed to persist keyring (retry in 1 second)")
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}
	} else if err != nil {
		return nil, fmt.Errorf("error loading keyring: %w", err)
	} else {
		lg.Warn("this agent has already been bootstrapped but may have been interrupted - will use existing keyring")
	}

	lg.Info("running post-bootstrap finalization steps")
	if err := a.bootstrapper.Finalize(ctx); err != nil {
		lg.With(zap.Error(err)).Warn("error in post-bootstrap finalization")
	} else {
		lg.Info("bootstrap completed successfully")
	}
	return a.loadKeyring(ctx)
}

func (a *Agent) loadKeyring(ctx context.Context) (keyring.Keyring, error) {
	lg := a.logger
	lg.Info("loading keyring")
	kr, err := a.keyringStore.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading keyring: %w", err)
	}
	lg.Info("keyring loaded successfully")
	return kr, nil
}

func (a *Agent) GetHealth(context.Context, *emptypb.Empty) (*corev1.Health, error) {
	conditions := []string{}
	a.conditions.Range(func(key string, value conditionStatus) bool {
		conditions = append(conditions, fmt.Sprintf("%s %s", key, value))
		return true
	})

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
	}, nil
}

func (a *Agent) initConditions() {
	a.conditions.Store(condRemoteWrite, statusPending)
	a.conditions.Store(condRuleSync, statusPending)
}

func (a *Agent) buildTrustStrategy(kr keyring.Keyring) (trust.Strategy, error) {
	var trustStrategy trust.Strategy
	var err error
	switch a.TrustStrategy {
	case v1beta1.TrustStrategyPKP:
		conf := trust.StrategyConfig{
			PKP: &trust.PKPConfig{
				Pins: trust.NewKeyringPinSource(kr),
			},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring pkp trust from keyring: %w", err)
		}
	case v1beta1.TrustStrategyCACerts:
		conf := trust.StrategyConfig{
			CACerts: &trust.CACertsConfig{
				CACerts: trust.NewKeyringCACertsSource(kr),
			},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring ca certs trust from keyring: %w", err)
		}
	case v1beta1.TrustStrategyInsecure:
		conf := trust.StrategyConfig{
			Insecure: &trust.InsecureConfig{},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring insecure trust: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown trust strategy: %s", a.TrustStrategy)
	}

	return trustStrategy, nil
}

func (a *Agent) GetKeyring(ctx context.Context) (keyring.Keyring, error) {
	return a.keyringStore.Get(ctx)
}

func (a *Agent) GetTrustStrategy() trust.Strategy {
	return a.trust
}

func (a *Agent) setCondition(key string, value conditionStatus, reason string) {
	lg := a.logger.With(
		"condition", key,
		"status", value,
		"reason", reason,
	)
	if v, ok := a.conditions.Load(key); ok {
		if v != value {
			lg.Info("condition changed")
		}
	} else {
		lg.Info("condition set")
	}
	a.conditions.Store(key, value)
}

func (a *Agent) clearCondition(key string, reason ...string) {
	lg := a.logger.With(
		"condition", key,
	)
	if len(reason) > 0 {
		lg = lg.With("reason", reason[0])
	}
	if v, ok := a.conditions.Load(key); ok {
		lg.With(
			"previous", v,
		).Info("condition cleared")
	}
	a.conditions.Delete(key)
}
