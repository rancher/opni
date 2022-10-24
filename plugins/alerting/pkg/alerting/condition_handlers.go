/*
- Functions that handle each endpoint implementation update case
- Functions that handle each alert condition case
*/
package alerting

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	natsutil "github.com/rancher/opni/pkg/util/nats"
)

func setupCondition(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1.AlertCondition, newConditionId string) (*corev1.Reference, error) {
	if s := req.GetAlertType().GetSystem(); s != nil {
		err := handleSystemAlertCreation(p, lg, ctx, s, newConditionId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if k := req.GetAlertType().GetKubeState(); k != nil {
		err := handleKubeAlertCreation(p, lg, ctx, k, newConditionId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
}

func deleteCondition(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1.AlertCondition, id string) error {
	if s := req.GetAlertType().GetSystem(); s != nil {
		p.msgNode.RemoveConfigListener(id)
		p.storageNode.DeleteAgentIncidentTracker(ctx, id)
		return nil
	}
	if k := req.GetAlertType().GetKubeState(); k != nil {
		_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: k.GetClusterId(),
			GroupName: CortexRuleIdFromUuid(id),
		})
		return err
	}
	return shared.AlertingErrNotImplemented
}

func handleSystemAlertCreation(
	p *Plugin,
	lg *zap.SugaredLogger,
	ctx context.Context,
	k *alertingv1.AlertConditionSystem,
	newConditionId string,
) error {
	caFunc := p.onSystemConditionCreate(newConditionId, k)
	p.msgNode.AddSystemConfigListener(newConditionId, caFunc)
	return nil
}

func handleKubeAlertCreation(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, k *alertingv1.AlertConditionKubeState, newId string) error {
	baseKubeRule, err := metrics.NewKubeStateRule(
		k.GetObjectType(),
		k.GetObjectName(),
		k.GetNamespace(),
		k.GetState(),
		timeDurationToPromStr(k.GetFor().AsDuration()),
		metrics.KubeStateAnnotations,
	)
	if err != nil {
		return err
	}
	kubeRuleContent, err := NewCortexAlertingRule(newId, nil, baseKubeRule)
	p.Logger.With("handler", "kubeStateAlertCreate").Debugf("kube state alert created %v", kubeRuleContent)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(kubeRuleContent)
	if err != nil {
		return err
	}
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   k.GetClusterId(),
		YamlContent: string(out),
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) onSystemConditionCreate(conditionId string, condition *alertingv1.AlertConditionSystem) context.CancelFunc {
	lg := p.Logger.With("onSystemConditionCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)

	// spawn async subscription stream
	go func() {
		defer cancel() // cancel parent context, if we return (non-recoverable)
		for {
			nc := p.natsConn.Get()
			js, err := nc.JetStream()
			if err != nil {
				lg.Error("failed to get jetstream context")
				continue
			}
			err = natsutil.NewPersistentStream(js, shared.NewAlertingDisconnectStream())
			if err != nil {
				lg.Errorf("alerting disconnect stream does not exist and cannot be created %s", err)
				continue
			}
			agentId := condition.GetClusterId().Id
			msgCh := make(chan *nats.Msg, 32)
			sub, err := js.ChanSubscribe(shared.NewAgentDisconnectSubject(agentId), msgCh)
			if err != nil {
				lg.Errorf("failed to chan subscribe %s", err)
			}
			defer sub.Unsubscribe()
			if err != nil {
				lg.Errorf("failed  to subscribe to %s : %s", shared.NewAgentDisconnectSubject(agentId), err)
				continue
			}
			for {
				select {
				case <-p.Ctx.Done():
					return
				case <-jsCtx.Done():
					return
				case msg := <-msgCh:
					var status health.StatusUpdate
					err := json.Unmarshal(msg.Data, &status)
					if err != nil {
						lg.Error(err)
					}
					p.storageNode.AddToAgentIncidentTracker(jsCtx, conditionId, alertstorage.AgentIncidentStep{
						StatusUpdate: status,
						AlertFiring:  false,
					})
				}
			}
		}
	}()
	// spawn a watcher for the incidents
	go func() {
		currentlyFiring := false
		defer cancel() // cancel parent context, if we return (non-recoverable)
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()
		for {
			select {
			case <-p.Ctx.Done():
				return
			case <-jsCtx.Done():
				return
			case <-ticker.C:
				st, err := p.storageNode.GetAgentIncidentTracker(jsCtx, conditionId)
				if err != nil || st == nil {
					lg.Error(err)
					continue
				}
				if len(st.Steps) == 0 {
					panic("no system alert condition steps")
				}
				a := st.Steps[len(st.Steps)-1]
				if !a.Status.Connected {
					interval := timestamppb.Now().AsTime().Sub(a.Status.Timestamp.AsTime())
					if interval > condition.GetTimeout().AsDuration() {
						_, err := p.TriggerAlerts(jsCtx, &alertingv1.TriggerAlertsRequest{
							ConditionId: &corev1.Reference{Id: conditionId},
							Annotations: map[string]string{
								shared.BackendConditionIdLabel: conditionId,
							},
						})
						if err != nil {
							lg.Error(err)
						}
						p.storageNode.AddToAgentIncidentTracker(jsCtx, conditionId, alertstorage.AgentIncidentStep{
							StatusUpdate: a.StatusUpdate, // must copy old timestamp
							AlertFiring:  true,
						})
						currentlyFiring = true
					} else {
						currentlyFiring = false
					}
				} else if a.Status.Connected && currentlyFiring {
					p.storageNode.AddToAgentIncidentTracker(jsCtx, conditionId, alertstorage.AgentIncidentStep{
						StatusUpdate: a.StatusUpdate, // must copy old timestamp
						AlertFiring:  false,
					})
					currentlyFiring = false
				}
			}
		}
	}()
	return cancel
}
