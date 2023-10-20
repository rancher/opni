package default_driver

import (
	"context"
	"fmt"
	"time"

	node_drivers "github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"

	"log/slog"

	promoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeDriverOptions struct {
	K8sClient client.Client `option:"k8sClient"`
	Logger    *slog.Logger  `option:"logger"`
}

type Driver struct {
	*NodeDriverOptions
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
	if err := d.K8sClient.List(ctx, rules, listOptions); err != nil {
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
					var dur time.Duration
					id := fmt.Sprintf("%s-%s-%s", parentUuid, ruleGroupId, rule.Alert)
					if rule.For != nil {
						var err error
						dur, err = time.ParseDuration(string(*rule.For))
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
					} else {
						dur = time.Minute
					}
				}
			}
		}
	}
	return alertingRules
}

func (d *Driver) ConfigureNode(_ string, _ *node.AlertingCapabilityConfig) error {
	return nil
}

func NewDriver(options *NodeDriverOptions) (*Driver, error) {
	if options.K8sClient == nil {
		s := scheme.Scheme
		promoperatorv1.AddToScheme(s)
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: s,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.K8sClient = c
	}
	return &Driver{
		NodeDriverOptions: options,
	}, nil
}

var _ node_drivers.NodeDriver = (*Driver)(nil)

func init() {
	node_drivers.NodeDrivers.Register("k8s_driver", func(ctx context.Context, opts ...driverutil.Option) (node_drivers.NodeDriver, error) {
		driverOptions := &NodeDriverOptions{
			K8sClient: nil,
			Logger:    logger.NewPluginLogger().WithGroup("alerting").WithGroup("rule-discovery"),
		}
		if err := driverutil.ApplyOptions(driverOptions, opts...); err != nil {
			return nil, err
		}
		return NewDriver(&NodeDriverOptions{})
	})
}
