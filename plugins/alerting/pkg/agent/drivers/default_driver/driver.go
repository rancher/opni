package default_driver

import (
	"context"
	"fmt"
	"time"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DriverOptions struct {
	k8sClient client.Client
}

type Driver struct {
	DriverOptions
}

func (d *Driver) DiscoverRules(ctx context.Context) (*rules.RuleManifest, error) {
	promRules, err := d.listRules(ctx)
	if err != nil {
		return nil, err
	}
	return d.filterAlertingRules(promRules), nil
}

func (d *Driver) listRules(ctx context.Context) (*promoperatorv1.PrometheusRuleList, error) {
	rules := &promoperatorv1.PrometheusRuleList{}
	listOptions := &client.ListOptions{
		Namespace: metav1.NamespaceAll,
	}
	if err := d.k8sClient.List(ctx, rules, listOptions); err != nil {
		return nil, err
	}
	return rules, nil
}

func (d *Driver) filterAlertingRules(promRules *promoperatorv1.PrometheusRuleList) *rules.RuleManifest {
	alertingRules := &rules.RuleManifest{
		Rules: []*rules.Rule{},
	}
	for _, operatorSpec := range promRules.Items {
		parentUuid := operatorSpec.UID // part of final uuid
		for _, ruleGroup := range operatorSpec.Spec.Groups {
			ruleGroupId := ruleGroup.Name
			for _, rule := range ruleGroup.Rules {
				if rule.Alert != "" {
					id := fmt.Sprintf("%s-%s-%s", parentUuid, ruleGroupId, rule.Alert)
					dur, err := time.ParseDuration(string(rule.For))
					if err != nil {
						dur = time.Minute
					}
					alertingRules.Rules = append(alertingRules.Rules, &rules.Rule{
						RuleId: &corev1.Reference{
							Id: id,
						},
						GroupId: &corev1.Reference{
							Id: ruleGroupId,
						},
						Name:        rule.Alert,
						Expr:        rule.Expr.String(),
						Duration:    durationpb.New(time.Duration(dur)),
						Labels:      rule.Labels,
						Annotations: rule.Annotations,
					})
				}
			}
		}
	}
	return alertingRules
}

func (d *Driver) ConfigureNode(_nodeId string, _ *node.AlertingCapabilityConfig) error {
	return nil
}

func NewDriver() *Driver {
	return &Driver{}
}

var _ drivers.NodeDriver = (*Driver)(nil)

func init() {
	drivers.NodeDrivers.Register("default_driver", func(_ context.Context, _ ...driverutil.Option) (drivers.NodeDriver, error) {
		return NewDriver(), nil
	})
}
