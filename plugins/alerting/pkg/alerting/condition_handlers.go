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
	"github.com/rancher/opni/pkg/validation"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
)

func setEndpointImplementationIfAvailable(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) error {
	lg = lg.With("function", "setEndpointImplementationIfAvailable")
	if req.NotificationId != nil { // create the endpoint implementation
		lg.Debug("creating notification implementation")
		if req.Details == nil {
			return validation.Error("alerting notification details must be set if you specify a notification target")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			lg.Error(err)
			return err
		}

		_, err := p.CreateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			EndpointId:     &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: newId},
			Implementation: req.Details,
		})
		if err != nil {
			lg.Error(err)
			return err
		}
	}
	return nil
}

func updateEndpointImplemetation(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
	lg = lg.With("function", "updateEndpointImplemetation")
	if req.NotificationId != nil { // create the endpoint implementation
		lg.Debugf("updating notification with id : %s", req.NotificationId)
		if req.Details == nil {
			return validation.Error("alerting notification details must be set")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			lg.Error(err)
			return err
		}

		_, err := p.UpdateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			EndpointId:     &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: id},
			Implementation: req.Details,
		})
		if err != nil {
			lg.Error(err)
			return err
		}
	}
	return nil
}

func handleUpdateEndpointImplementation(
	p *Plugin,
	lg *zap.SugaredLogger,
	ctx context.Context,
	conditionId string,
	existing *alertingv1alpha.AlertCondition,
	new *alertingv1alpha.AlertCondition,
) error {
	lg = lg.With("function", "handleUpdateEndpointImplementation")
	lg.Debugf("handling update endpoint implementation on condition id %s", conditionId)
	if existing.NotificationId != nil { // delete implementation
		// !!! must pass in the existing condition id
		lg.Debug("previous implementation set, removing...")
		_, err := p.DeleteEndpointImplementation(ctx, &corev1.Reference{Id: conditionId})
		return err
	} else {
		lg.Debug("no previous implementation set")
	}
	lg.Debug("updating implementation")
	return setEndpointImplementationIfAvailable(p, lg, ctx, new, conditionId)
}

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
