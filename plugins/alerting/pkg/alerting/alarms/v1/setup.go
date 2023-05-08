package alarms

import (
	"bytes"
	"context"

	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/metrics/naming"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

func (p *AlarmServerComponent) setupCondition(
	ctx context.Context,
	_ *zap.SugaredLogger,
	req *alertingv1.AlertCondition,
	newConditionId string) (*corev1.Reference, error) {
	if req.GetAlertType().GetSystem() != nil {
		err := p.handleSystemAlertCreation(ctx, req, newConditionId, req.GetName(), req.Namespace())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetDownstreamCapability() != nil {
		err := p.handleDownstreamCapabilityAlertCreation(ctx, req, newConditionId, req.GetName(), req.Namespace())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetMonitoringBackend() != nil {
		err := p.handleMonitoringBackendAlertCreation(ctx, req, newConditionId, req.GetName(), req.Namespace())
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetKubeState() != nil {
		err := p.handleKubeAlertCreation(ctx, req, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.GetAlertType().GetCpu() != nil {
		err := p.handleCpuSaturationAlertCreation(ctx, req, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.AlertType.GetMemory() != nil {
		err := p.handleMemorySaturationAlertCreation(ctx, req, newConditionId, req.Name)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.AlertType.GetFs() != nil {
		if err := p.handleFsSaturationAlertCreation(ctx, req, newConditionId, req.Name); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	if req.AlertType.GetPrometheusQuery() != nil {
		if err := p.handlePrometheusQueryAlertCreation(ctx, req, newConditionId, req.Name); err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newConditionId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
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
		p.logger.Errorf("failed to create agent condition %s", err)
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
		p.logger.Errorf("failed to create agent condition %s", err)
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
		p.logger.Errorf("failed to create cortex cluster condition %s", err)
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
	p.logger.With("handler", "kubeStateAlertCreate").Debugf("kube state alert created %v", kubeRuleContent)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(kubeRuleContent)
	if err != nil {
		return err
	}
	p.logger.With("Expr", "kube-state").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
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
	p.logger.With("Expr", "cpu").Debugf("%s", string(out))

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
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
	p.logger.With("Expr", "mem").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
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
	p.logger.With("Expr", "fs").Debugf("%s", string(out))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
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
	p.logger.With("Expr", "user-query").Debugf("%s", string(out.Bytes()))
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		ClusterId:   q.ClusterId.GetId(),
		Namespace:   shared.OpniAlertingCortexNamespace,
		YamlContent: out.Bytes(),
	})

	return err
}
