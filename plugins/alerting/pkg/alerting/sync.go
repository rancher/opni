package alerting

import (
	"context"
	"sync"
	"time"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (p *Plugin) createDefaultDisconnect(ctx context.Context, clusterId string) error {
	conditions, err := p.storageNode.Get().Conditions.List(p.Ctx, alertstorage.WithUnredacted())
	if err != nil {
		p.Logger.Errorf("failed to list alert conditions : %s", err)
		return err
	}
	disconnectExists := false
	for _, cond := range conditions {
		if s := cond.GetAlertType().GetSystem(); s != nil {
			if s.GetClusterId().Id == clusterId {
				disconnectExists = true
				break
			}
		}
	}
	if disconnectExists {
		return nil
	}
	_, err = p.CreateAlertCondition(p.Ctx, &alertingv1.AlertCondition{
		Name:        "agent-disconnect",
		Description: "Alert when the downstream agent disconnects from the opni upstream",
		Labels:      []string{"agent-disconnect", "opni", "automatic"},
		Severity:    alertingv1.Severity_CRITICAL,
		AlertType: &alertingv1.AlertTypeDetails{
			Type: &alertingv1.AlertTypeDetails_System{
				System: &alertingv1.AlertConditionSystem{
					ClusterId: &corev1.Reference{Id: clusterId},
					Timeout:   durationpb.New(10 * time.Minute),
				},
			},
		},
	})
	if err != nil {
		p.Logger.Warnf(
			"could not create a downstream agent disconnect condition  on cluster creation for cluster %s",
			clusterId,
		)
	} else {
		p.Logger.Debugf(
			"downstream agent disconnect condition on cluster creation for cluster %s is now active",
			clusterId,
		)
	}
	return nil
}

func (p *Plugin) onDeleteClusterAgentDisconnectHook(ctx context.Context, clusterId string) error {
	conditions, err := p.storageNode.Get().Conditions.List(p.Ctx, alertstorage.WithUnredacted())
	if err != nil {
		p.Logger.Errorf("failed to list conditions from storage : %s", err)
	}
	var wg sync.WaitGroup
	for _, cond := range conditions {
		cond := cond
		if s := cond.GetAlertType().GetSystem(); s != nil {
			if s.GetClusterId().Id == clusterId {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err = p.DeleteAlertCondition(ctx, &corev1.Reference{
						Id: cond.Id,
					})
					if err != nil {
						p.Logger.Errorf("failed to delete condition %s : %s", cond.Id, err)
					}
				}()
			}
		}
	}
	wg.Wait()
	return nil
}

func (p *Plugin) createDefaultCapabilityHealth(ctx context.Context, clusterId string) error {
	items, err := p.ListAlertConditions(p.Ctx, &alertingv1.ListAlertConditionRequest{})
	if err != nil {
		p.Logger.Errorf("failed to list alert conditions : %s", err)
		return err
	}
	healthExists := false
	for _, item := range items.Items {
		if s := item.GetAlertCondition().GetAlertType().GetDownstreamCapability(); s != nil {
			if s.GetClusterId().Id == clusterId {
				healthExists = true
				break
			}
		}
	}

	if healthExists {
		return nil
	}

	_, err = p.CreateAlertCondition(p.Ctx, &alertingv1.AlertCondition{
		Name:        "agent-capability-unhealthy",
		Description: "Alert when some downstream agent capability becomes unhealthy",
		Labels:      []string{"agent-capability-health", "opni", "automatic"},
		Severity:    alertingv1.Severity_CRITICAL,
		AlertType: &alertingv1.AlertTypeDetails{
			Type: &alertingv1.AlertTypeDetails_DownstreamCapability{
				DownstreamCapability: &alertingv1.AlertConditionDownstreamCapability{
					ClusterId:       &corev1.Reference{Id: clusterId},
					CapabilityState: ListBadDefaultStatuses(),
					For:             durationpb.New(10 * time.Minute),
				},
			},
		},
	})
	if err != nil {
		p.Logger.Warnf(
			"could not create a default downstream capability health condition on cluster creation for cluster %s",
			clusterId,
		)
	} else {
		p.Logger.Debugf(
			"downstream agent disconnect condition on cluster creation for cluster %s is now active",
			clusterId,
		)
	}
	return nil
}

func (p *Plugin) onDeleteClusterCapabilityHook(ctx context.Context, clusterId string) error {
	conditions, err := p.storageNode.Get().Conditions.List(p.Ctx, alertstorage.WithUnredacted())
	if err != nil {
		p.Logger.Errorf("failed to list conditions from storage : %s", err)
	}
	var wg sync.WaitGroup
	for _, cond := range conditions {
		cond := cond
		if dc := cond.GetAlertType().GetDownstreamCapability(); dc != nil {
			if dc.ClusterId.Id == clusterId {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err = p.DeleteAlertCondition(ctx, &corev1.Reference{
						Id: cond.Id,
					})
					if err != nil {
						p.Logger.Errorf("failed to delete condition %s : %s", cond.Id, err)
					}
				}()
			}
		}
	}
	wg.Wait()
	return nil
}
