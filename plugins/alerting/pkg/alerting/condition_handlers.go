/*
- Functions that handle each endpoint implementation update case
- Functions that handle each alert condition case
*/
package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
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
		err := p.handleSystemAlertCreation(ctx, s, newConditionId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if k := req.GetAlertType().GetKubeState(); k != nil {
		err := p.handleKubeAlertCreation(ctx, k, newConditionId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if c := req.GetAlertType().GetCpu(); c != nil {
		err := p.handleCpuSaturationAlertCreation(ctx, c, newConditionId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if m := req.AlertType.GetMemory(); m != nil {
		err := p.handleMemorySaturationAlertCreation(ctx, m, newConditionId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if fs := req.AlertType.GetFs(); fs != nil {
		if err := p.handleFsSaturationAlertCreation(ctx, fs, newConditionId); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
}

func deleteCondition(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1.AlertCondition, id string) error {
	switch r := req.GetAlertType(); {
	case r.GetSystem() != nil:
		p.msgNode.RemoveConfigListener(id)
		p.storageNode.DeleteAgentIncidentTracker(ctx, id)
		return nil
	case r.GetKubeState() != nil || r.GetCpu() != nil || r.GetMemory() != nil || r.GetFs() != nil:
		_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: handleSwitchCortexRules(r).Id,
			GroupName: CortexRuleIdFromUuid(id),
		})
		return err
	}
	return shared.AlertingErrNotImplemented
}

func handleSwitchCortexRules(t *alertingv1.AlertTypeDetails) *corev1.Reference {
	if k := t.GetKubeState(); k != nil {
		return &corev1.Reference{Id: k.ClusterId}
	}
	if c := t.GetCpu(); c != nil {
		return c.ClusterId
	}
	if m := t.GetMemory(); m != nil {
		return m.ClusterId
	}
	if f := t.GetFs(); f != nil {
		return f.ClusterId
	}

	return nil
}

func (p *Plugin) handleSystemAlertCreation(
	ctx context.Context,
	k *alertingv1.AlertConditionSystem,
	newConditionId string,
) error {
	_, err := p.storageNode.GetAgentIncidentTracker(ctx, newConditionId)
	if errors.Is(err, nats.ErrKeyNotFound) {
		p.storageNode.CreateAgentIncidentTracker(ctx, newConditionId, alertstorage.AgentIncidentStep{
			StatusUpdate: health.StatusUpdate{
				ID: k.GetClusterId().Id,
				Status: &corev1.Status{
					Timestamp: nil,
					Connected: true,
				},
			},
			AlertFiring: false,
		})
	} else if err != nil {
		return err
	}
	caFunc := p.onSystemConditionCreate(newConditionId, k)
	p.msgNode.AddSystemConfigListener(newConditionId, caFunc)
	return nil
}

func (p *Plugin) handleKubeAlertCreation(ctx context.Context, k *alertingv1.AlertConditionKubeState, newId string) error {
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
	p.Logger.With("Expr", "kube-state").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   k.GetClusterId(),
		YamlContent: string(out),
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) handleCpuSaturationAlertCreation(ctx context.Context, c *alertingv1.AlertConditionCPUSaturation, conditionId string) error {
	baseCpuRule, err := metrics.NewCpuRule(
		c.GetNodeCoreFilters(),
		c.GetCpuStates(),
		c.GetOperation(),
		float64(c.GetExpectedRatio()),
		c.GetFor(),
		metrics.CpuRuleAnnotations,
	)
	if err != nil {
		return err
	}
	cpuRuleContent, err := NewCortexAlertingRule(conditionId, nil, baseCpuRule)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cpuRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "cpu").Debugf("%s", string(out))

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   c.ClusterId.GetId(),
		YamlContent: string(out),
	})
	return err
}

func (p *Plugin) handleMemorySaturationAlertCreation(ctx context.Context, m *alertingv1.AlertConditionMemorySaturation, conditionId string) error {
	baseMemRule, err := metrics.NewMemRule(
		m.GetNodeMemoryFilters(),
		m.UsageTypes,
		m.GetOperation(),
		float64(m.GetExpectedRatio()),
		m.GetFor(),
		metrics.MemRuleAnnotations,
	)
	if err != nil {
		return err
	}
	memRuleContent, err := NewCortexAlertingRule(conditionId, nil, baseMemRule)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(memRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "mem").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   m.ClusterId.GetId(),
		YamlContent: string(out),
	})
	return err
}

func (p *Plugin) handleFsSaturationAlertCreation(ctx context.Context, fs *alertingv1.AlertConditionFilesystemSaturation, conditionId string) error {
	baseFsRule, err := metrics.NewFsRule(
		fs.GetNodeFilters(),
		fs.GetOperation(),
		float64(fs.GetExpectedRatio()),
		fs.GetFor(),
		metrics.MemRuleAnnotations,
	)
	if err != nil {
		return err
	}
	fsRuleContent, err := NewCortexAlertingRule(conditionId, nil, baseFsRule)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(fsRuleContent)
	if err != nil {
		return err
	}
	p.Logger.With("Expr", "fs").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		ClusterId:   fs.ClusterId.GetId(),
		YamlContent: string(out),
	})
	return err
}

func (p *Plugin) onSystemConditionCreate(conditionId string, condition *alertingv1.AlertConditionSystem) context.CancelFunc {
	lg := p.Logger.With("onSystemConditionCreate", conditionId)
	lg.Debugf("received condition update: %v", condition)
	jsCtx, cancel := context.WithCancel(p.Ctx)
	var firingLock sync.RWMutex
	currentlyFiring := false

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
					firingLock.RLock()
					err = p.storageNode.AddToAgentIncidentTracker(jsCtx, conditionId, alertstorage.AgentIncidentStep{
						StatusUpdate: status,
						AlertFiring:  currentlyFiring,
					})
					if err != nil {
						lg.Error(err)
					}
					firingLock.RUnlock()
				}
			}
		}
	}()
	// spawn a watcher for the incidents
	go func() {
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
					lg.Warn(err)
					continue
				}
				if len(st.Steps) == 0 {
					panic("no system alert condition steps")
				}
				a := st.Steps[len(st.Steps)-1]
				if a.Status.Timestamp == nil {
					continue
				}
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
						firingLock.Lock()
						currentlyFiring = true
						firingLock.Unlock() // in case Updating the incident tracker takes a while, but we still want to process incoming messages
						firingLock.RLock()
						p.storageNode.AddToAgentIncidentTracker(jsCtx, conditionId, alertstorage.AgentIncidentStep{
							StatusUpdate: a.StatusUpdate, // must copy old timestamp
							AlertFiring:  currentlyFiring,
						})
						firingLock.RUnlock()
					} else {
						currentlyFiring = false
					}
				} else if a.Status.Connected && currentlyFiring {
					firingLock.Lock()
					currentlyFiring = false
					firingLock.Unlock()
					firingLock.RLock()
					p.storageNode.AddToAgentIncidentTracker(jsCtx, conditionId, alertstorage.AgentIncidentStep{
						StatusUpdate: a.StatusUpdate, // must copy old timestamp
						AlertFiring:  currentlyFiring,
					})
					firingLock.RUnlock()
				}
			}
		}
	}()
	return cancel
}
