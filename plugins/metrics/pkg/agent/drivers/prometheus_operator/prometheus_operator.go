package prometheus_operator

import (
	"context"
	"fmt"
	"os"

	"log/slog"

	"github.com/lestrrat-go/backoff/v2"
	monitoringcoreosv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringcoreosv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/rules/prometheusrule"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	reconcilerutil "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers/util"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExternalPromOperatorDriver struct {
	ExternalPromOperatorDriverOptions
	state reconcilerutil.ReconcilerState
}

type ExternalPromOperatorDriverOptions struct {
	K8sClient client.Client `option:"k8sClient"`
	Logger    *slog.Logger  `option:"logger"`
	Namespace string        `option:"namespace"`
}

func NewExternalPromOperatorDriver(options ExternalPromOperatorDriverOptions) (*ExternalPromOperatorDriver, error) {
	if options.K8sClient == nil {
		s := scheme.Scheme
		monitoringcoreosv1.AddToScheme(s)
		monitoringcoreosv1alpha1.AddToScheme(s)
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: s,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.K8sClient = c
	}
	driver := &ExternalPromOperatorDriver{
		ExternalPromOperatorDriverOptions: options,
	}
	return driver, nil
}

func (d *ExternalPromOperatorDriver) ConfigureNode(nodeId string, conf *node.MetricsCapabilityStatus) error {
	lg := d.Logger.With("nodeId", nodeId)
	if d.state.GetRunning() {
		d.state.Cancel()
	}
	d.state.SetRunning(true)
	ctx, ca := context.WithCancel(context.TODO())
	d.state.SetBackoffCtx(ctx, ca)

	var deployPrometheus bool
	if conf.Enabled {
		if conf.GetSpec().Driver == nil {
			// old config from before the driver field was added
			deployPrometheus = conf.GetSpec().GetPrometheus() != nil
		} else {
			deployPrometheus = conf.GetSpec().GetPrometheus() != nil &&
				conf.GetSpec().GetDriver() == node.MetricsCapabilityConfig_Prometheus
		}
	}

	objList := []reconcilerutil.ReconcileItem{}
	svcAccount, cr, crb := d.buildRbac()
	scrapeConfigs := d.buildAdditionalScrapeConfigsSecret()
	prometheus := d.buildPrometheus()
	prometheusAgent := d.buildPrometheusAgent(conf.GetSpec().GetPrometheus())
	objList = append(objList, reconcilerutil.ReconcileItem{
		A: svcAccount,
		B: deployPrometheus,
	}, reconcilerutil.ReconcileItem{
		A: cr,
		B: deployPrometheus,
	}, reconcilerutil.ReconcileItem{
		A: crb,
		B: deployPrometheus,
	}, reconcilerutil.ReconcileItem{
		A: prometheus,
		B: false,
	}, reconcilerutil.ReconcileItem{
		A: prometheusAgent,
		B: deployPrometheus,
	}, reconcilerutil.ReconcileItem{
		A: scrapeConfigs,
		B: deployPrometheus,
	})
	p := backoff.Exponential()
	b := p.Start(ctx)
	var success bool
BACKOFF:
	for backoff.Continue(b) {
		for _, obj := range objList {
			lg.Debug(fmt.Sprintf("object : %s, should exist : %t", client.ObjectKeyFromObject(obj.A).String(), obj.B))
			if err := reconcilerutil.ReconcileObject(lg, d.K8sClient, d.Namespace, obj); err != nil {
				lg.With(
					"object", client.ObjectKeyFromObject(obj.A).String(),
					logger.Err(err),
				).Error("error reconciling object")
				continue BACKOFF
			}
		}
		success = true
		break
	}

	if !success {
		lg.Error("timed out reconciling objects")
		return fmt.Errorf("timed out reconciling objects")
	}
	lg.Info("objects reconciled successfully")
	return nil
}

func (d *ExternalPromOperatorDriver) buildPrometheus() *monitoringcoreosv1.Prometheus {
	return &monitoringcoreosv1.Prometheus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-prometheus-agent",
			Namespace: d.Namespace,
		},
	}
}

func (d *ExternalPromOperatorDriver) buildPrometheusAgent(conf *node.PrometheusSpec) *monitoringcoreosv1alpha1.PrometheusAgent {
	image := "quay.io/prometheus/prometheus:latest"
	if conf.GetImage() != "" {
		image = conf.GetImage()
	}

	selector := &metav1.LabelSelector{}

	return &monitoringcoreosv1alpha1.PrometheusAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-prometheus-agent",
			Namespace: d.Namespace,
		},
		Spec: monitoringcoreosv1alpha1.PrometheusAgentSpec{
			CommonPrometheusFields: monitoringcoreosv1.CommonPrometheusFields{
				Image:    &image,
				LogLevel: "debug",
				RemoteWrite: []monitoringcoreosv1.RemoteWriteSpec{
					{
						URL: fmt.Sprintf("http://%s.%s.svc/api/agent/push", d.serviceName(), d.Namespace),
						// Default queue config:
						//   MaxShards:         200,
						//   MinShards:         1,
						//   MaxSamplesPerSend: 500,
						//   Capacity:          2500
						//   BatchSendDeadline: 5s
						//   MinBackoff:        30ms
						//   MaxBackoff:        5s
						//
						// Default target max bandwidth: 500 samples * 200 shards * 10 requests/shard/s = 1M samples/s
						// Default capacity goal = 600k samples
						QueueConfig: &monitoringcoreosv1.QueueConfig{
							MaxShards:         100,
							MinShards:         1,
							MaxSamplesPerSend: 1000,
							Capacity:          5000,
							BatchSendDeadline: "2s",
							MinBackoff:        "5s",
							MaxBackoff:        "1m",
							RetryOnRateLimit:  true,
							MaxRetries:        15,
						},
					},
				},
				ScrapeInterval:                  "30s",
				Replicas:                        lo.ToPtr[int32](1),
				PodMonitorNamespaceSelector:     selector,
				PodMonitorSelector:              selector,
				ProbeNamespaceSelector:          selector,
				ProbeSelector:                   selector,
				ServiceMonitorNamespaceSelector: selector,
				ServiceMonitorSelector:          selector,
				ServiceAccountName:              "opni-prometheus-agent",
				AdditionalScrapeConfigs: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "opni-additional-scrape-configs",
					},
					Key: "prometheus.yaml",
				},
			},
		},
	}
}

func (d *ExternalPromOperatorDriver) buildAdditionalScrapeConfigsSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-additional-scrape-configs",
			Namespace: d.Namespace,
		},
		Data: map[string][]byte{
			"prometheus.yaml": []byte(`
- job_name: "prometheus"
  static_configs:
  - targets: ["localhost:9090"]
`[1:]),
		},
	}
}

func (d *ExternalPromOperatorDriver) serviceName() string {
	list := &corev1.ServiceList{}
	err := d.K8sClient.List(context.TODO(), list,
		client.InNamespace(d.Namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	)
	if err != nil {
		d.Logger.Error("unable to list services, defaulting to opni-agent")
		return "opni-agent"
	}
	if len(list.Items) != 1 {
		d.Logger.Error("unable to fetch service name, defaulting to opni-agent")
		return "opni-agent"
	}
	return list.Items[0].Name
}

func (d *ExternalPromOperatorDriver) buildRbac() (*corev1.ServiceAccount, *rbacv1.ClusterRole, *rbacv1.ClusterRoleBinding) {
	svcAcct := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-prometheus-agent",
			Namespace: d.Namespace,
		},
	}
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-prometheus-agent-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{
					"nodes",
					"nodes/metrics",
					"services",
					"endpoints",
					"pods",
				},
				Verbs: []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				NonResourceURLs: []string{"/metrics"},
				Verbs:           []string{"get"},
			},
		},
	}
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-prometheus-agent-rb",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "opni-prometheus-agent-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "opni-prometheus-agent",
				Namespace: d.Namespace,
			},
		},
	}
	return svcAcct, clusterRole, clusterRoleBinding
}

func (d *ExternalPromOperatorDriver) DiscoverPrometheuses(ctx context.Context, namespace string) ([]*remoteread.DiscoveryEntry, error) {
	list := &monitoringcoreosv1.PrometheusList{}
	if err := d.K8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	return lo.Map(list.Items, func(prom *monitoringcoreosv1.Prometheus, _ int) *remoteread.DiscoveryEntry {
		return &remoteread.DiscoveryEntry{
			Name:             prom.Name,
			ClusterId:        "", // populated by the gateway
			ExternalEndpoint: prom.Spec.ExternalURL,
			InternalEndpoint: fmt.Sprintf("%s.%s.svc.cluster.local", prom.Name, prom.Namespace),
		}
	}), nil
}

func (d *ExternalPromOperatorDriver) ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup] {
	if config.Discovery.PrometheusRules != nil {
		opts := []prometheusrule.PrometheusRuleFinderOption{prometheusrule.WithLogger(d.Logger)}
		if len(config.Discovery.PrometheusRules.SearchNamespaces) > 0 {
			opts = append(opts, prometheusrule.WithNamespaces(config.Discovery.PrometheusRules.SearchNamespaces...))
		}
		return prometheusrule.NewPrometheusRuleFinder(d.K8sClient, opts...)
	}
	return nil
}

func init() {
	drivers.NodeDrivers.Register("prometheus-operator", func(_ context.Context, opts ...driverutil.Option) (drivers.MetricsNodeDriver, error) {
		options := ExternalPromOperatorDriverOptions{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Logger:    logger.NewPluginLogger().WithGroup("metrics").WithGroup("prometheus-operator"),
		}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}
		return NewExternalPromOperatorDriver(options)
	})
}
