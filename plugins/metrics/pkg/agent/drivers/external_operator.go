package drivers

import (
	"context"
	"fmt"
	"os"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	monitoringcoreosv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/samber/lo"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExternalPromOperatorDriver struct {
	ExternalPromOperatorDriverOptions

	logger    *zap.SugaredLogger
	namespace string
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
		c, err := util.NewK8sClient(util.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.k8sClient = c
	}

	return &ExternalPromOperatorDriver{
		ExternalPromOperatorDriverOptions: options,
		logger:                            logger,
		namespace:                         namespace,
	}, nil
}

var _ MetricsNodeDriver = (*ExternalPromOperatorDriver)(nil)

func (*ExternalPromOperatorDriver) Name() string {
	return "external-operator"
}

func (d *ExternalPromOperatorDriver) ConfigureNode(conf *node.MetricsCapabilityConfig) error {
	prometheus := d.buildPrometheus(conf.GetSpec().GetPrometheus())
	svcAccount, cr, crb := d.buildRbac()

	shouldExist := conf.Enabled && conf.GetSpec().GetPrometheus().GetDeploymentStrategy() == "externalPromOperator"

	return retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		for _, obj := range []client.Object{prometheus, svcAccount, cr, crb} {
			if err := d.reconcileObject(obj, shouldExist); err != nil {
				return err
			}
		}
		return nil
	})
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
						},
					},
				},
				RemoteWrite: []monitoringcoreosv1.RemoteWriteSpec{
					{
						URL: fmt.Sprintf("http://opni-agent.%s.svc/api/agent/push", d.namespace),
					},
				},
				Replicas:                        lo.ToPtr[int32](1),
				PodMonitorNamespaceSelector:     selector,
				PodMonitorSelector:              selector,
				ProbeNamespaceSelector:          selector,
				ProbeSelector:                   selector,
				ServiceMonitorNamespaceSelector: selector,
				ServiceMonitorSelector:          selector,
				ServiceAccountName:              "opni-prometheus-agent",
			},
		},
	}
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

	current := desired.DeepCopyObject().(client.Object)
	err := d.k8sClient.Get(context.TODO(), key, current)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	owner := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "opni-agent",
	}
	desired.SetOwnerReferences([]metav1.OwnerReference{owner})

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
