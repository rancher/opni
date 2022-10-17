/*
- Functions that handle each endpoint implementation update case
- Functions that handle each alert condition case
*/
package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
)

func setupCondition(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1alpha.AlertCondition, newConditionId string) (*corev1.Reference, error) {
	if s := req.GetAlertType().GetSystem(); s != nil {
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

func deleteCondition(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
	if s := req.GetAlertType().GetSystem(); s != nil {
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

func handleKubeAlertCreation(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, k *alertingv1alpha.AlertConditionKubeState, newId string) error {
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
