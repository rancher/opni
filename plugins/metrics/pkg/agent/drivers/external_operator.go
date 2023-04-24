package drivers

import (
	"context"
	"errors"
	"fmt"
	"os"

	driverutil "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/lestrrat-go/backoff/v2"
	monitoringcoreosv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/samber/lo"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ExternalPromOperatorDriver struct {
	ExternalPromOperatorDriverOptions

	logger    *zap.SugaredLogger
	namespace string
	state     driverutil.ReconcilerState
}

type ExternalPromOperatorDriverOptions struct {
	k8sClient client.Client
}

type ExternalPromOperatorDriverOption func(*ExternalPromOperatorDriverOptions)

func (o *ExternalPromOperatorDriverOptions) apply(opts ...ExternalPromOperatorDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithK8sClient(k8sClient client.Client) ExternalPromOperatorDriverOption {
	return func(o *ExternalPromOperatorDriverOptions) {
		o.k8sClient = k8sClient
	}
}

func NewExternalPromOperatorDriver(
	logger *zap.SugaredLogger,
	opts ...ExternalPromOperatorDriverOption,
) (*ExternalPromOperatorDriver, error) {
	options := ExternalPromOperatorDriverOptions{}
	options.apply(opts...)

	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("POD_NAMESPACE environment variable not set")
	}

	if options.k8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.k8sClient = c
	}
	driver := &ExternalPromOperatorDriver{
		ExternalPromOperatorDriverOptions: options,
		logger:                            logger,
		namespace:                         namespace,
	}
	return driver, nil
}

var _ MetricsNodeDriver = (*ExternalPromOperatorDriver)(nil)

func (d *ExternalPromOperatorDriver) ConfigureNode(_ string, conf *node.MetricsCapabilityConfig) {
	if d.state.GetRunning() {
		d.state.Cancel()
	}
	d.state.SetRunning(true)
	ctx, ca := context.WithCancel(context.TODO())
	d.state.SetBackoffCtx(ctx, ca)

	// deployOtel := conf.Enabled && features.FeatureList.FeatureIsEnabled(FeatureFlagOtel)
	deployPrometheus := conf.Enabled &&
		conf.GetSpec().GetPrometheus() != nil &&
		conf.GetSpec().GetPrometheus().GetDeploymentStrategy() == "externalPromOperator"

	objList := []driverutil.ReconcileItem{}
	svcAccount, cr, crb := d.buildRbac()
	scrapeConfigs := d.buildAdditionalScrapeConfigsSecret()
	prometheus := d.buildPrometheus(conf.GetSpec().GetPrometheus())
	objList = append(objList, driverutil.ReconcileItem{
		A: svcAccount,
		B: deployPrometheus,
	}, driverutil.ReconcileItem{
		A: cr,
		B: deployPrometheus,
	}, driverutil.ReconcileItem{
		A: crb,
		B: deployPrometheus,
	}, driverutil.ReconcileItem{
		A: prometheus,
		B: deployPrometheus,
	}, driverutil.ReconcileItem{
		A: scrapeConfigs,
		B: deployPrometheus,
	})
	p := backoff.Exponential()
	b := p.Start(ctx)
	var success bool
BACKOFF:
	for backoff.Continue(b) {
		for _, obj := range objList {
			d.logger.Debugf("object : %s, should exist : %t", client.ObjectKeyFromObject(obj.A).String(), obj.B)
			if err := driverutil.ReconcileObject(d.logger, d.k8sClient, d.namespace, obj); err != nil {
				d.logger.With(
					"object", client.ObjectKeyFromObject(obj.A).String(),
					zap.Error(err),
				).Error("error reconciling object")
				continue BACKOFF
			}
		}
		success = true
		break
	}

	if !success {
		d.logger.Error("timed out reconciling objects")
	} else {
		d.logger.Info("objects reconciled successfully")
	}
}

func (d *ExternalPromOperatorDriver) buildPrometheus(conf *node.PrometheusSpec) *monitoringcoreosv1.Prometheus {
	image := "quay.io/prometheus/prometheus:latest"
	if conf.GetImage() != "" {
		image = conf.GetImage()
	}

	selector := &metav1.LabelSelector{}

	return &monitoringcoreosv1.Prometheus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-prometheus-agent",
			Namespace: d.namespace,
		},
		Spec: monitoringcoreosv1.PrometheusSpec{
			CommonPrometheusFields: monitoringcoreosv1.CommonPrometheusFields{
				Image: &image,
				Containers: []corev1.Container{
					{
						Name: "prometheus",
						Args: []string{
							"--config.file=/etc/prometheus/config_out/prometheus.env.yaml",
							"--web.enable-lifecycle",
							"--storage.agent.path=/prometheus",
							"--enable-feature=agent",
							"--log.level=debug",
						},
					},
				},
				RemoteWrite: []monitoringcoreosv1.RemoteWriteSpec{
					{
						URL: fmt.Sprintf("http://%s.%s.svc/api/agent/push", d.serviceName(), d.namespace),
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
			Namespace: d.namespace,
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
	err := d.k8sClient.List(context.TODO(), list,
		client.InNamespace(d.namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	)
	if err != nil {
		d.logger.Error("unable to list services, defaulting to opni-agent")
		return "opni-agent"
	}
	if len(list.Items) != 1 {
		d.logger.Error("unable to fetch service name, defaulting to opni-agent")
		return "opni-agent"
	}
	return list.Items[0].Name
}

func (d *ExternalPromOperatorDriver) buildRbac() (*corev1.ServiceAccount, *rbacv1.ClusterRole, *rbacv1.ClusterRoleBinding) {
	svcAcct := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-prometheus-agent",
			Namespace: d.namespace,
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
				Namespace: d.namespace,
			},
		},
	}
	return svcAcct, clusterRole, clusterRoleBinding
}

func (d *ExternalPromOperatorDriver) reconcileObject(desired client.Object, shouldExist bool) error {
	// get the object
	key := client.ObjectKeyFromObject(desired)
	lg := d.logger.With("object", key)
	lg.Info("reconciling object")

	// get the agent statefulset
	list := &appsv1.StatefulSetList{}
	if err := d.k8sClient.List(context.TODO(), list,
		client.InNamespace(d.namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	); err != nil {
		return err
	}

	if len(list.Items) != 1 {
		return errors.New("statefulsets found not exactly 1")
	}
	agentStatefulSet := &list.Items[0]

	current := desired.DeepCopyObject().(client.Object)
	err := d.k8sClient.Get(context.TODO(), key, current)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	// this can error if the object is cluster-scoped, but that's ok
	controllerutil.SetOwnerReference(agentStatefulSet, desired, d.k8sClient.Scheme())

	if k8serrors.IsNotFound(err) {
		if !shouldExist {
			lg.Info("object does not exist and should not exist, skipping")
			return nil
		}
		lg.Info("object does not exist, creating")
		// create the object
		return d.k8sClient.Create(context.TODO(), desired)
	} else if !shouldExist {
		// delete the object
		lg.Info("object exists and should not exist, deleting")
		return d.k8sClient.Delete(context.TODO(), current)
	}

	// update the object
	patchResult, err := patch.DefaultPatchMaker.Calculate(current, desired, patch.IgnoreStatusFields())
	if err != nil {
		d.logger.With(
			zap.Error(err),
		).Warn("could not match objects")
		return err
	}
	if patchResult.IsEmpty() {
		d.logger.Info("resource is in sync")
		return nil
	}
	d.logger.Info("resource diff")

	if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(desired); err != nil {
		d.logger.With(
			zap.Error(err),
		).Error("failed to set last applied annotation")
	}

	metaAccessor := meta.NewAccessor()

	currentResourceVersion, err := metaAccessor.ResourceVersion(current)
	if err != nil {
		return err
	}
	if err := metaAccessor.SetResourceVersion(desired, currentResourceVersion); err != nil {
		return err
	}

	d.logger.Info("updating resource")

	return d.k8sClient.Update(context.TODO(), desired)
}

func (d *ExternalPromOperatorDriver) DiscoverPrometheuses(ctx context.Context, namespace string) ([]*remoteread.DiscoveryEntry, error) {
	list := &monitoringcoreosv1.PrometheusList{}
	if err := d.k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
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
