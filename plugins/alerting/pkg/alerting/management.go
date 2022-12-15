package alerting

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"

	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// capability name ---> condition name ---> condition status
var registerMu sync.RWMutex
var RegisteredCapabilityStatuses = map[string]map[string][]health.ConditionStatus{}

func RegisterCapabilityStatus(capabilityName, condName string, availableStatuses []health.ConditionStatus) {
	registerMu.Lock()
	defer registerMu.Unlock()
	if _, ok := RegisteredCapabilityStatuses[capabilityName]; !ok {
		RegisteredCapabilityStatuses[capabilityName] = map[string][]health.ConditionStatus{}
	}
	RegisteredCapabilityStatuses[capabilityName][condName] = availableStatuses
}

func ListCapabilityStatuses(capabilityName string) map[string][]health.ConditionStatus {
	registerMu.RLock()
	defer registerMu.RUnlock()
	return RegisteredCapabilityStatuses[capabilityName]
}

func ListBadDefaultStatuses() []string {
	return []string{health.StatusFailure.String(), health.StatusPending.String()}
}

func init() {
	// metrics
	RegisterCapabilityStatus(
		wellknown.CapabilityMetrics,
		health.CondConfigSync,
		[]health.ConditionStatus{health.StatusPending, health.StatusFailure})
	RegisterCapabilityStatus(
		wellknown.CapabilityMetrics,
		agent.CondRemoteWrite,
		[]health.ConditionStatus{health.StatusPending, health.StatusFailure})
	RegisterCapabilityStatus(
		wellknown.CapabilityMetrics,
		agent.CondRuleSync,
		[]health.ConditionStatus{
			health.StatusPending,
			health.StatusFailure})
	RegisterCapabilityStatus(
		wellknown.CapabilityMetrics,
		health.CondBackend,
		[]health.ConditionStatus{health.StatusPending, health.StatusFailure})
	//logging
	RegisterCapabilityStatus(wellknown.CapabilityLogs, health.CondConfigSync, []health.ConditionStatus{
		health.StatusPending,
		health.StatusFailure,
	})
	RegisterCapabilityStatus(wellknown.CapabilityLogs, health.CondBackend, []health.ConditionStatus{
		health.StatusPending,
		health.StatusFailure,
	})
}

func (p *Plugin) configureAlertManagerConfiguration(pluginCtx context.Context, opts ...drivers.AlertingManagerDriverOption) {
	// load default cluster drivers
	drivers.ResetClusterDrivers()
	if kcd, err := drivers.NewAlertingManagerDriver(opts...); err == nil {
		drivers.RegisterClusterDriver(kcd)
	} else {
		drivers.LogClusterDriverFailure(kcd.Name(), err) // Name() is safe to call on a nil pointer
	}

	name := "alerting-mananger"
	driver, err := drivers.GetClusterDriver(name)
	if err != nil {
		p.Logger.With(
			"driver", name,
			zap.Error(err),
		).Error("failed to load cluster driver, using fallback no-op driver")
		if os.Getenv(shared.LocalBackendEnvToggle) != "" {
			driver = drivers.NewLocalManager(
				drivers.WithLocalManagerLogger(p.Logger),
				drivers.WithLocalManagerContext(pluginCtx),
			)

		} else {
			driver = &drivers.NoopClusterDriver{}
		}
	}
	p.opsNode.ClusterDriver.Set(driver)
}

// blocking
func (p *Plugin) watchModelTrainingStatus() {
	lg := p.Logger.With("watcher", "model-training-status")
	err := natsutil.NewPersistentStream(p.js.Get(), NewModelTrainingStatusStream())
	if err != nil {
		panic(err)
	}
	//acquire model training plugin client
	var adminClient modeltraining.ModelTrainingClient
	for {
		ctxca, ca := context.WithTimeout(p.Ctx, 5*time.Second)
		acquiredClient, err := p.modeltrainingClient.GetContext(ctxca)
		ca()
		if err != nil {
			lg.Warn("could not acquire model training client within timeout, retrying...")
		} else {
			adminClient = acquiredClient
			break
		}
	}
	lg.Debug(adminClient)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.Ctx.Done():
			lg.Debug("closing model training status watcher")
			return
		case <-ticker.C:
			mStatus, err := adminClient.GetModelStatus(p.Ctx, &emptypb.Empty{})
			if err != nil {
				continue
			}
			go func() {
				modelTrainingStatusData, err := json.Marshal(mStatus)
				if err != nil {
					p.Logger.Errorf("failed to marshal cortex cluster status: %s", err)
				}
				_, err = p.js.Get().PublishAsync(NewModelTrainingStatusSubject(mStatus.Statistics.Uuid), modelTrainingStatusData)
				if err != nil {
					p.Logger.Errorf("failed to publish cortex cluster status : %s", err)
				}
			}()
		}
	}
}

// blocking
func (p *Plugin) watchCortexClusterStatus() {
	lg := p.Logger.With("watcher", "cortex-cluster-status")
	err := natsutil.NewPersistentStream(p.js.Get(), NewCortexStatusStream())
	if err != nil {
		panic(err)
	}
	// acquire cortex client
	var adminClient cortexadmin.CortexAdminClient
	for {
		ctxca, ca := context.WithTimeout(p.Ctx, 5*time.Second)
		acquiredClient, err := p.adminClient.GetContext(ctxca)
		ca()
		if err != nil {
			lg.Warn("could not acquire cortex admin client within timeout, retrying...")
		} else {
			adminClient = acquiredClient
			break
		}
	}

	ticker := time.NewTicker(60 * time.Second) // making this more fine-grained is not necessary
	defer ticker.Stop()
	for {
		select {
		case <-p.Ctx.Done():
			lg.Debug("closing cortex cluster status watcher...")
		case <-ticker.C:
			ccStatus, err := adminClient.GetCortexStatus(p.Ctx, &emptypb.Empty{})
			if err != nil {
				if e, ok := status.FromError(err); ok {
					switch e.Code() {
					case codes.Unavailable:
						lg.Debugf("Cortex cluster status unavailable : not yet installed")
						continue
					case codes.Internal:
						if ccStatus == nil {
							ccStatus = &cortexadmin.CortexStatus{}
						}
						// status is so badly messed up we can assume nothing is working
						// mark all sub-statues as nil so they are always evaluated as unhealthy
					case codes.Unknown: // this might be a blip, but mark this as unhealthy for everything
						ccStatus = &cortexadmin.CortexStatus{}
						lg.Warnf("Cortex cluster status unknown : %v", err)
						continue
					}
				}
			}
			go func() {
				cortexStatusData, err := json.Marshal(ccStatus)
				if err != nil {
					p.Logger.Errorf("failed to marshal cortex cluster status: %s", err)
				}
				_, err = p.js.Get().PublishAsync(NewCortexStatusSubject(), cortexStatusData)
				if err != nil {
					p.Logger.Errorf("failed to publish cortex cluster status : %s", err)
				}
			}()
		}
	}
}

// blocking
func (p *Plugin) watchGlobalCluster(
	client managementv1.ManagementClient,
	watcher *ManagementWatcherHooks[*managementv1.WatchEvent],
) {
	clusterClient, err := client.WatchClusters(p.Ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		p.Logger.Error("failed to watch clusters, exiting...")
		os.Exit(1)
	}
	for {
		select {
		case <-p.Ctx.Done():
			return
		default:
			event, err := clusterClient.Recv()
			if err != nil {
				p.Logger.Errorf("failed to receive cluster event : %s", err)
			}
			watcher.HandleEvent(event)
		}
	}
}

// blocking
func (p *Plugin) watchGlobalClusterHealthStatus(client managementv1.ManagementClient, ingressStream *nats.StreamConfig) {
	err := natsutil.NewPersistentStream(p.js.Get(), ingressStream)
	if err != nil {
		panic(err)
	}
	clusterStatusClient, err := client.WatchClusterHealthStatus(p.Ctx, &emptypb.Empty{})
	if err != nil {
		p.Logger.Error("failed to watch cluster health status, exiting...")
		os.Exit(1)
	}
	// on startup always send a manual read in case the gateway was down when the agent status changed
	cls, err := client.ListClusters(p.Ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		p.Logger.Error("failed to list clusters, exiting...")
		os.Exit(1)
	}
	for _, cl := range cls.Items {
		clusterStatus, err := client.GetClusterHealthStatus(p.Ctx, &corev1.Reference{Id: cl.GetId()})
		//make sure durable consumer is setup
		replayErr := natsutil.NewDurableReplayConsumer(p.js.Get(), ingressStream.Name, NewAgentDurableReplayConsumer(cl.GetId()))
		if replayErr != nil {
			panic(replayErr)
		}
		if err == nil {
			clusterStatusData, err := json.Marshal(clusterStatus)
			if err != nil {
				p.Logger.Errorf("failed to marshal cluster health status: %s", err)
				continue
			}

			_, err = p.js.Get().PublishAsync(ingressStream.Name, clusterStatusData)
			if err != nil {
				p.Logger.Errorf("failed to publish cluster health status : %s", err)
			}
		} else {
			p.Logger.Warnf("failed to read cluster health status on startup for cluster %s : %s", cl.GetId(), err.Error())
		}
	}
	for {
		select {
		case <-p.Ctx.Done():
			return
		default:
			clusterStatus, err := clusterStatusClient.Recv()
			if err != nil {
				p.Logger.Warn("failed to receive cluster health status from grpc stream, retrying...")
				continue
			}
			clusterStatusData, err := json.Marshal(clusterStatus)
			if err != nil {
				p.Logger.Errorf("failed to marshal cluster health status: %s", err)
				continue
			}
			_, err = p.js.Get().PublishAsync(NewAgentStreamSubject(clusterStatus.Cluster.Id), clusterStatusData)
			if err != nil {
				p.Logger.Errorf("failed to publish cluster health status : %s", err)
			}
		}
	}
}
