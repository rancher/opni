package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
func (p *Plugin) watchGlobalCluster(client managementv1.ManagementClient) {
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
			switch event.Type {
			case managementv1.WatchEventType_Created:
				_, err := p.CreateAlertCondition(p.Ctx, &alertingv1.AlertCondition{
					Name: fmt.Sprintf("agent-disconnect (%s)", event.Cluster.Id),
					Description: fmt.Sprintf(
						"Alert when the downstream agent (%s) disconnects from the opni upstream",
						event.Cluster.Id,
					),
					Labels:   []string{"agent-disconnect", "opni"},
					Severity: alertingv1.Severity_CRITICAL,
					AlertType: &alertingv1.AlertTypeDetails{
						Type: &alertingv1.AlertTypeDetails_System{
							System: &alertingv1.AlertConditionSystem{
								ClusterId: event.Cluster.Reference(),
								Timeout:   durationpb.New(10 * time.Minute),
							},
						},
					},
				})
				if err != nil {
					p.Logger.Warn(
						"could not create a downstream agent disconnect condition  on cluster creation for cluster %s",
						event.Cluster.Id,
					)
				} else {
					p.Logger.Debug(
						"downstream agent disconnect condition on cluster creation for cluster %s is now active",
						event.Cluster.Id,
					)
				}
			case managementv1.WatchEventType_Deleted:
				// delete any conditions that are associated with this cluster
				ids, conds, err := p.storageNode.ListWithKeyConditionStorage(p.Ctx)
				if err != nil {
					p.Logger.Errorf("failed to list conditions from storage : %s", err)
				}
				for i, id := range ids {
					if s := conds[i].GetAlertType().GetSystem(); s != nil {
						if s.ClusterId.Id == event.Cluster.Id {
							_, err = p.DeleteAlertCondition(p.Ctx, &corev1.Reference{
								Id: id,
							})
							if err != nil {
								p.Logger.Errorf("failed to delete condition %s : %s", id, err)
							}
						}
					}
				}
			}
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
			msg := &health.StatusUpdate{
				ID:     clusterStatus.Cluster.Id,
				Status: clusterStatus.HealthStatus.Status,
			}
			data, err := json.Marshal(msg)
			if err != nil {
				p.Logger.Errorf("failed to marshal cluster health status update : %s", err)
			}
			_, err = js.PublishAsync(shared.NewAgentDisconnectSubject(clusterStatus.Cluster.GetId()), data)
			if err != nil {
				p.Logger.Errorf("failed to publish cluster health status update : %s", err)
			}
		}
	}
}
