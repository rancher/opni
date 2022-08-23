/*
- Functions that handle each endpoint implementation update case
- Functions that handle each alert condition case
*/
package alerting

import (
	"context"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
)

func setEndpointImplementationIfAvailable(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) error {
	if req.NotificationId != nil { // create the endpoint implementation
		if req.Details == nil {
			return validation.Error("alerting notification details must be set if you specify a notification target")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			return err
		}

		_, err := p.CreateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			EndpointId:     &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: newId},
			Implementation: req.Details,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func updateEndpointImplemetation(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
	if req.NotificationId != nil { // create the endpoint implementation
		if req.Details == nil {
			return validation.Error("alerting notification details must be set")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			return err
		}

		_, err := p.UpdateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			EndpointId:     &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: id},
			Implementation: req.Details,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func handleUpdateEndpointImplementation(
	p *Plugin,
	ctx context.Context,
	id string,
	existing *alertingv1alpha.AlertCondition,
	new *alertingv1alpha.AlertCondition,
) error {
	if existing.NotificationId == nil { // no implementation previously set
		return setEndpointImplementationIfAvailable(p, ctx, new, id)
	} else if new.NotificationId == nil { // delete implementation
		_, err := p.DeleteEndpointImplementation(ctx, &corev1.Reference{Id: *existing.NotificationId})
		return err
	}
	return updateEndpointImplemetation(p, ctx, new, id)
}

func setupCondition(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) (*corev1.Reference, error) {
	if s := req.GetAlertType().GetSystem(); s != nil {
		return &corev1.Reference{Id: newId}, nil
	}
	if k := req.GetAlertType().GetKubeState(); k != nil {
		err := handleKubeAlertCreation(p, ctx, k, newId)
		if err != nil {
			return nil, err
		}
		return &corev1.Reference{Id: newId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
}

func deleteCondition(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
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

func handleKubeAlertCreation(p *Plugin, ctx context.Context, k *alertingv1alpha.AlertConditionKubeState, newId string) error {
	baseKubeRule, err := metrics.NewKubePodStateRule(
		k.GetObject(),
		k.GetNamespace(),
		k.GetState(),
		timeDurationToPromStr(k.GetFor().AsDuration()),
		nil, // FIXME : make a cortex receiver that calls HandleCortexWebhook, then pass in appropriate labels
		nil, //FIXME : make a cortex receiver that calls HandleCortexWebhook, then pass in appropriate annotations
	)
	if err != nil {
		return err
	}
	kubeRuleContent, err := NewCortexAlertingRule(newId, nil, baseKubeRule)
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
