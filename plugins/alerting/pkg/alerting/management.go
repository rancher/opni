package alerting

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"

	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
func (p *Plugin) watchCortexClusterStatus() {
	lg := p.Logger.With("watcher", "cortex-cluster-status")
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
			status, err := adminClient.GetCortexStatus(p.Ctx, &emptypb.Empty{})
			if err != nil {
				lg.Debugf("failed to get cortex cluster status %s", err)
				continue
			}
			for _, cortexMsgReceiver := range p.msgNode.ListCortexStatusListeners() {
				select {
				case cortexMsgReceiver <- status:
				default:
				}
			}
		}
	}
}

// blocking
func (p *Plugin) watchGlobalCluster(
	client managementv1.ManagementClient,
	watcher *ClusterWatcherHooks[*managementv1.WatchEvent],
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
func (p *Plugin) watchGlobalClusterHealthStatus(client managementv1.ManagementClient) {
	clusterStatusClient, err := client.WatchClusterHealthStatus(p.Ctx, &emptypb.Empty{})
	if err != nil {
		p.Logger.Error("failed to watch cluster health status, exiting...")
		os.Exit(1)
	}
	p.Logger.Debug("acquiring jetstream context for global health status stream...")
	conn := p.natsConn.Get()
	js, err := conn.JetStream()
	if err != nil {
		p.Logger.Error("failed to acquire jetstream context for global health status stream, exiting...")
		os.Exit(1)
	}
	p.Logger.Debug("acquired jetstream context for global health status stream")
	for {
		select {
		case <-p.Ctx.Done():
			return
		default:
			clusterStatus, err := clusterStatusClient.Recv()
			if err != nil {
				p.Logger.Error()
			}
			if clusterStatus.HealthStatus == nil { // isn't clear if this should be explicitly checked
				continue
			}
			if clusterStatus.HealthStatus.Health == nil {
				clusterStatus.HealthStatus.Health = &corev1.Health{
					Timestamp: timestamppb.Now(),
					Ready:     false,
				}
			}
			if clusterStatus.HealthStatus.Health.Timestamp == nil {
				clusterStatus.HealthStatus.Health.Timestamp = timestamppb.Now()
			}
			msg := &health.StatusUpdate{
				ID:     clusterStatus.Cluster.Id,
				Status: clusterStatus.HealthStatus.Status,
			}
			// send to agent disconnect
			go func() {
				agentDisconnectData, err := json.Marshal(msg)
				if err != nil {
					p.Logger.Errorf("failed to marshal cluster health status update : %s", err)
				}
				_, err = js.PublishAsync(shared.NewAgentDisconnectSubject(clusterStatus.Cluster.GetId()), agentDisconnectData)
				if err != nil {
					p.Logger.Errorf("failed to publish cluster health status update : %s", err)
				}
			}()

			// send to health status
			go func() {
				healthStatusData, err := json.Marshal(clusterStatus)
				if err != nil {
					p.Logger.Errorf("failed to marshal cluster health status update : %s", err)
				}
				_, err = js.PublishAsync(shared.NewHealthStatusSubject(clusterStatus.Cluster.GetId()), healthStatusData)
				if err != nil {
					p.Logger.Errorf("failed to publish cluster health status update : %s", err)
				}
			}()
		}
	}
}
