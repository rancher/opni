package alarms

import (
	"bytes"
	"context"
	"fmt"

	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/metrics/naming"
	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"gopkg.in/yaml.v3"
)

const (
	metadataLastAppliedHashKey = "opni.io/alarm-hash"
	metadataInactiveAlarm      = "opni.io/alarm-inactive"
)

func (p *AlarmServerComponent) shouldDelete(
	cond *alertingv1.AlertCondition,
) bool {
	if cond.GetMetadata() != nil && cond.GetMetadata()[metadataCleanUpAlarm] != "" {
		return true
	}
	return false
}

func (p *AlarmServerComponent) hasChanged(
	ctx context.Context,
	newCond *alertingv1.AlertCondition,
	conditionId string,
) (bool, error) {
	condStorage, err := p.conditionStorage.GetContext(ctx)
	if err != nil {
		return false, err
	}
	cond, err := condStorage.Group(newCond.GroupId).Get(ctx, conditionId)
	if err == nil {
		if lastAppliedHash, ok := cond.GetMetadata()[metadataLastAppliedHashKey]; ok {
			configHash, err := newCond.Hash()
			if err != nil {
				return false, err
			}
			if lastAppliedHash == configHash {
				return false, nil
			}
		}
	}
	return true, nil
}

func (p *AlarmServerComponent) requiresDeploy(
	ctx context.Context,
	cond *alertingv1.AlertCondition,
	conditionId string,
) bool {
	if cond.GetMetadata() != nil && cond.GetMetadata()[metadataInactiveAlarm] != "" {
		return true
	}

	status, err := p.AlertConditionStatus(ctx, &alertingv1.ConditionReference{Id: conditionId, GroupId: cond.GroupId})
	if err == nil {
		if status.State != alertingv1.AlertConditionState_Invalidated {
			return false
		}
	}
	return true
}

func (p *AlarmServerComponent) applyAlarm(
	ctx context.Context,
	cond *alertingv1.AlertCondition,
	conditionId string,
	syncInfo alertingSync.SyncInfo,
) (ref *corev1.Reference, retErr error) {
	shouldDelete := p.shouldDelete(cond)
	if shouldDelete {
		return &corev1.Reference{Id: conditionId}, p.teardownCondition(ctx, cond, conditionId, true)
	}

	cond.Visit(syncInfo)
	hasChanged, err := p.hasChanged(ctx, cond, conditionId)
	if err != nil {
		return nil, err
	}
	isInvalid := p.requiresDeploy(ctx, cond, conditionId)
	shouldDeploy := isInvalid || hasChanged
	if !shouldDeploy {
		return
	}
	return p.activateCondition(
		ctx,
		cond,
		conditionId,
	)
}

func (p *AlarmServerComponent) activateCondition(
	ctx context.Context,
	cond *alertingv1.AlertCondition,
	conditionId string,
) (ref *corev1.Reference, retErr error) {
	conditionStorage, err := p.conditionStorage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr == nil {
			configHash, err := cond.Hash()
			if err != nil {
				retErr = err
				return
			}
			md := func() map[string]string {
				md := cond.GetMetadata()
				if md == nil {
					return map[string]string{}
				}
				return md
			}()
			md[metadataLastAppliedHashKey] = configHash
			delete(md, metadataInactiveAlarm)
			cond.Metadata = md
			retErr = conditionStorage.Group(cond.GroupId).Put(ctx, conditionId, cond)
		}
	}()

	if cond.GetAlertType().GetSystem() != nil {
		retErr = p.handleSystemAlertCreation(ctx, cond, conditionId, cond.GetName(), cond.Namespace())
	}
	if cond.GetAlertType().GetDownstreamCapability() != nil {
		retErr = p.handleDownstreamCapabilityAlertCreation(ctx, cond, conditionId, cond.GetName(), cond.Namespace())
	}
	if cond.GetAlertType().GetMonitoringBackend() != nil {
		retErr = p.handleMonitoringBackendAlertCreation(ctx, cond, conditionId, cond.GetName(), cond.Namespace())
	}
	if cond.GetAlertType().GetKubeState() != nil {
		retErr = p.handleKubeAlertCreation(ctx, cond, conditionId, cond.Name)
	}
	if cond.GetAlertType().GetCpu() != nil {
		retErr = p.handleCpuSaturationAlertCreation(ctx, cond, conditionId, cond.Name)
	}
	if cond.AlertType.GetMemory() != nil {
		retErr = p.handleMemorySaturationAlertCreation(ctx, cond, conditionId, cond.Name)
		return &corev1.Reference{Id: conditionId}, nil
	}
	if cond.AlertType.GetFs() != nil {
		retErr = p.handleFsSaturationAlertCreation(ctx, cond, conditionId, cond.Name)
	}
	if cond.AlertType.GetPrometheusQuery() != nil {
		retErr = p.handlePrometheusQueryAlertCreation(ctx, cond, conditionId, cond.Name)
	}
	ref = &corev1.Reference{Id: conditionId}
	return
}

func (p *AlarmServerComponent) handleSystemAlertCreation(
	_ context.Context,
	k *alertingv1.AlertCondition,
	newConditionId string,
	conditionName string,
	namespace string,
) error {
	err := p.onSystemConditionCreate(newConditionId, conditionName, namespace, k)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to create agent condition %s", err))
	}
	return nil
}

func (p *AlarmServerComponent) handleDownstreamCapabilityAlertCreation(
	_ context.Context,
	k *alertingv1.AlertCondition,
	newConditionId string,
	conditionName string,
	namespace string,
) error {
	err := p.onDownstreamCapabilityConditionCreate(newConditionId, conditionName, namespace, k)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to create agent condition %s", err))
	}
	return nil
}

func (p *AlarmServerComponent) handleMonitoringBackendAlertCreation(
	_ context.Context,
	k *alertingv1.AlertCondition,
	newConditionId string,
	conditionName string,
	namespace string,
) error {
	err := p.onCortexClusterStatusCreate(newConditionId, conditionName, namespace, k)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to create cortex cluster condition %s", err))
	}
	return nil
}

func (p *AlarmServerComponent) handleKubeAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, newId, alertName string) error {
	k := cond.GetAlertType().GetKubeState()
	baseKubeRule, err := metrics.NewKubeStateRule(
		k.GetObjectType(),
		k.GetObjectName(),
		k.GetNamespace(),
		k.GetState(),
		cortex.TimeDurationToPromStr(k.GetFor().AsDuration()),
		naming.KubeStateAnnotations,
	)
	if err != nil {
		return err
	}
	kubeRuleContent, err := cortex.NewPrometheusAlertingRule(newId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		k, nil, baseKubeRule,
	)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(kubeRuleContent)
	if err != nil {
		return err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		return err
	}
	_, err = adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   k.GetClusterId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *AlarmServerComponent) handleCpuSaturationAlertCreation(
	ctx context.Context,
	cond *alertingv1.AlertCondition,
	conditionId,
	alertName string,
) error {
	c := cond.GetAlertType().GetCpu()
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
	cpuRuleContent, err := cortex.NewPrometheusAlertingRule(conditionId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		c, nil, baseCpuRule)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cpuRuleContent)
	if err != nil {
		return err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		return err
	}

	_, err = adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   c.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	return err
}

func (p *AlarmServerComponent) handleMemorySaturationAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, conditionId, alertName string) error {
	m := cond.GetAlertType().GetMemory()
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
	memRuleContent, err := cortex.NewPrometheusAlertingRule(conditionId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		m,
		nil,
		baseMemRule,
	)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(memRuleContent)
	if err != nil {
		return err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		return err
	}
	_, err = adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   m.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	return err
}

func (p *AlarmServerComponent) handleFsSaturationAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, conditionId, alertName string) error {
	fs := cond.GetAlertType().GetFs()
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
	fsRuleContent, err := cortex.NewPrometheusAlertingRule(
		conditionId,
		alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		fs,
		nil,
		baseFsRule,
	)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(fsRuleContent)
	if err != nil {
		return err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		return err
	}
	_, err = adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   fs.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out,
	})
	return err
}

func (p *AlarmServerComponent) handlePrometheusQueryAlertCreation(ctx context.Context, cond *alertingv1.AlertCondition, conditionId, alertName string) error {
	q := cond.GetAlertType().GetPrometheusQuery()
	dur := model.Duration(q.GetFor().AsDuration())
	baseRule := &metrics.AlertingRule{
		Alert:       "",
		Expr:        metrics.PostProcessRuleString(q.GetQuery()),
		For:         dur,
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	baseRuleContent, err := cortex.NewPrometheusAlertingRule(conditionId, alertName,
		cond.GetRoutingLabels(),
		cond.GetRoutingAnnotations(),
		q, nil, baseRule)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	err = encoder.Encode(baseRuleContent)
	if err != nil {
		return err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		return err
	}
	_, err = adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   q.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out.Bytes(),
	})

	return err
}
