package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/storage/opts"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"google.golang.org/protobuf/types/known/durationpb"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	metricsnode "github.com/rancher/opni/plugins/metrics/apis/node"
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
		metricsnode.CondRemoteWrite,
		[]health.ConditionStatus{health.StatusPending, health.StatusFailure})
	RegisterCapabilityStatus(
		wellknown.CapabilityMetrics,
		metricsnode.CondRuleSync,
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

var (
	DefaultDisconnectAlarm = func(clusterId string) *alertingv1.AlertCondition {
		return &alertingv1.AlertCondition{
			Name:        "agent-disconnect",
			Description: "Alert when the downstream agent disconnects from the opni upstream",
			Labels:      []string{"agent-disconnect", "opni", "_default"},
			Severity:    alertingv1.OpniSeverity_Critical,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{Id: clusterId},
						Timeout:   durationpb.New(10 * time.Minute),
					},
				},
			},
		}
	}

	DefaultCapabilityHealthAlarm = func(clusterId string) *alertingv1.AlertCondition {
		return &alertingv1.AlertCondition{
			Name:        "agent-capability-unhealthy",
			Description: "Alert when some downstream agent capability becomes unhealthy",
			Labels:      []string{"agent-capability-health", "opni", "_default"},
			Severity:    alertingv1.OpniSeverity_Critical,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_DownstreamCapability{
					DownstreamCapability: &alertingv1.AlertConditionDownstreamCapability{
						ClusterId:       &corev1.Reference{Id: clusterId},
						CapabilityState: ListBadDefaultStatuses(),
						For:             durationpb.New(10 * time.Minute),
					},
				},
			},
		}
	}
)

func (p *Plugin) createDefaultDisconnect(clusterId string) error {
	conditions, err := p.storageClientSet.Get().Conditions().Group("").List(p.ctx, opts.WithUnredacted())
	if err != nil {
		p.logger.Errorf("failed to list alert conditions : %s", err)
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
	_, err = p.CreateAlertCondition(p.ctx, DefaultDisconnectAlarm(clusterId))
	if err != nil {
		p.logger.Warnf(
			"could not create a downstream agent disconnect condition  on cluster creation for cluster %s",
			clusterId,
		)
	} else {
		p.logger.Debugf(
			"downstream agent disconnect condition on cluster creation for cluster %s is now active",
			clusterId,
		)
	}
	return nil
}

func (p *Plugin) onDeleteClusterAgentDisconnectHook(ctx context.Context, clusterId string) error {
	conditions, err := p.storageClientSet.Get().Conditions().Group("").List(p.ctx, opts.WithUnredacted())
	if err != nil {
		p.logger.Errorf("failed to list conditions from storage : %s", err)
	}
	var wg sync.WaitGroup
	for _, cond := range conditions {
		cond := cond
		if s := cond.GetAlertType().GetSystem(); s != nil {
			if s.GetClusterId().Id == clusterId {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err = p.DeleteAlertCondition(ctx, &alertingv1.ConditionReference{
						Id: cond.Id,
					})
					if err != nil {
						p.logger.Errorf("failed to delete condition %s : %s", cond.Id, err)
					}
				}()
			}
		}
	}
	wg.Wait()
	return nil
}

func (p *Plugin) createDefaultCapabilityHealth(clusterId string) error {
	items, err := p.storageClientSet.Get().Conditions().Group("").List(p.ctx, opts.WithUnredacted())
	if err != nil {
		p.logger.Errorf("failed to list alert conditions : %s", err)
		return err
	}
	healthExists := false
	for _, item := range items {
		if s := item.GetAlertType().GetDownstreamCapability(); s != nil {
			if s.GetClusterId().Id == clusterId {
				healthExists = true
				break
			}
		}
	}

	if healthExists {
		return nil
	}

	_, err = p.CreateAlertCondition(p.ctx, DefaultCapabilityHealthAlarm(clusterId))
	if err != nil {
		p.logger.Warnf(
			"could not create a default downstream capability health condition on cluster creation for cluster %s",
			clusterId,
		)
	} else {
		p.logger.Debugf(
			"downstream agent disconnect condition on cluster creation for cluster %s is now active",
			clusterId,
		)
	}
	return nil
}

func (p *Plugin) onDeleteClusterCapabilityHook(ctx context.Context, clusterId string) error {
	conditions, err := p.storageClientSet.Get().Conditions().Group("").List(p.ctx, opts.WithUnredacted())
	if err != nil {
		p.logger.Errorf("failed to list conditions from storage : %s", err)
	}
	var wg sync.WaitGroup
	for _, cond := range conditions {
		cond := cond
		if dc := cond.GetAlertType().GetDownstreamCapability(); dc != nil {
			if dc.ClusterId.Id == clusterId {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err = p.DeleteAlertCondition(ctx, &alertingv1.ConditionReference{
						Id: cond.Id,
					})
					if err != nil {
						p.logger.Errorf("failed to delete condition %s : %s", cond.Id, err)
					}
				}()
			}
		}
	}
	wg.Wait()
	return nil
}
