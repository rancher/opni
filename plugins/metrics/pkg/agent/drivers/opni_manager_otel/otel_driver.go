package opni_manager_otel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/apis"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	reconcilerutil "github.com/rancher/opni/plugins/metrics/pkg/agent/drivers/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/samber/lo"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OTELNodeDriver struct {
	OTELNodeDriverOptions

	logger    *zap.SugaredLogger
	namespace string
	state     reconcilerutil.ReconcilerState
}

func (*OTELNodeDriver) ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup] {
	return notifier.NewMultiFinder[rules.RuleGroup]() // TODO: implement
}

var _ drivers.MetricsNodeDriver = (*OTELNodeDriver)(nil)

type OTELNodeDriverOptions struct {
	K8sClient client.Client      `option:"k8sClient"`
	Logger    *zap.SugaredLogger `option:"logger"`
	Namespace string             `option:"namespace"`
}

func NewOTELDriver(options OTELNodeDriverOptions) (*OTELNodeDriver, error) {
	if options.K8sClient == nil {
		c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
			Scheme: apis.NewScheme(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
		options.K8sClient = c
	}

	return &OTELNodeDriver{
		OTELNodeDriverOptions: options,
		state:                 reconcilerutil.ReconcilerState{},
	}, nil
}

func (o *OTELNodeDriver) ConfigureNode(_ string, conf *node.MetricsCapabilityConfig) {
	if o.state.GetRunning() {
		o.state.Cancel()
	}
	o.state.SetRunning(true)
	ctx, ca := context.WithCancel(context.TODO())
	o.state.SetBackoffCtx(ctx, ca)

	deployOTEL := conf.Enabled &&
		conf.GetSpec().GetOtel() != nil

	otelConfig := o.buildMonitoringCollectorConfig(conf.GetSpec().GetOtel())
	objList := []reconcilerutil.ReconcileItem{
		{
			A: otelConfig,
			B: deployOTEL,
		},
	}
	p := backoff.Exponential()
	b := p.Start(ctx)
	var success bool
BACKOFF:
	for backoff.Continue(b) {
		for _, obj := range objList {
			o.logger.Debugf(
				"object : %s, should exist : %t",
				client.ObjectKeyFromObject(obj.A).String(),
				obj.B,
			)
			if err := reconcilerutil.ReconcileObject(o.logger, o.K8sClient, o.namespace, obj); err != nil {
				o.logger.With(
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
		o.logger.Error("timed out reconciling objects")
	} else {
		o.logger.Info("objects reconciled successfully")
	}
	o.logger.Info("starting collector reconcile...")
	if err := o.reconcileCollector(deployOTEL); err != nil {
		o.logger.With(
			"object", "opni collector",
			zap.Error(err),
		).Error("error reconciling object")
	}
	o.logger.Info("collector reconcile complete")
}

// no-op
func (o *OTELNodeDriver) DiscoverPrometheuses(_ context.Context, _ string) ([]*remoteread.DiscoveryEntry, error) {
	return []*remoteread.DiscoveryEntry{}, nil
}

func (o *OTELNodeDriver) buildMonitoringCollectorConfig(
	incomingSpec *node.OTELSpec,
) *monitoringv1beta1.CollectorConfig {
	collectorConfig := &monitoringv1beta1.CollectorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: otel.MetricsCrdName,
		},
		Spec: monitoringv1beta1.CollectorConfigSpec{
			PrometheusDiscovery: monitoringv1beta1.PrometheusDiscovery{},
			RemoteWriteEndpoint: o.getRemoteWriteEndpoint(),
			OtelSpec:            lo.FromPtrOr(node.CompatOTELStruct(incomingSpec), otel.OTELSpec{}),
		},
	}
	o.logger.Debugf("building %s", string(util.Must(json.Marshal(collectorConfig))))
	return collectorConfig
}

func (o *OTELNodeDriver) reconcileCollector(shouldExist bool) error {
	o.logger.Debug("reconciling collector")
	coll := &opnicorev1beta1.Collector{
		ObjectMeta: metav1.ObjectMeta{
			Name: otel.CollectorName,
		},
	}
	err := o.K8sClient.Get(context.TODO(), client.ObjectKeyFromObject(coll), coll)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if k8serrors.IsNotFound(err) && shouldExist {
		coll = o.buildEmptyCollector()
		coll.Spec.MetricsConfig = &corev1.LocalObjectReference{
			Name: otel.MetricsCrdName,
		}
		o.logger.Debug("creating collector with metrics config")
		return o.K8sClient.Create(context.TODO(), coll)
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		o.logger.Debug("updating collector with metrics config")
		err := o.K8sClient.Get(context.TODO(), client.ObjectKeyFromObject(coll), coll)
		if err != nil {
			return err
		}
		if shouldExist {
			o.logger.Debug("setting metrics config")
			coll.Spec.MetricsConfig = &corev1.LocalObjectReference{
				Name: otel.MetricsCrdName,
			}
		} else {
			o.logger.Debug("removing metrics config")
			coll.Spec.MetricsConfig = nil
		}
		return o.K8sClient.Update(context.TODO(), coll)
	})
	return err
}

func (o *OTELNodeDriver) buildEmptyCollector() *opnicorev1beta1.Collector {
	var serviceName string
	service, err := o.getAgentService()
	if err == nil {
		serviceName = service.Name
	}
	return &opnicorev1beta1.Collector{
		ObjectMeta: metav1.ObjectMeta{
			Name: otel.CollectorName,
		},
		Spec: opnicorev1beta1.CollectorSpec{
			ImageSpec: opnimeta.ImageSpec{
				ImagePullPolicy: lo.ToPtr(corev1.PullAlways),
			},
			SystemNamespace: o.namespace,
			AgentEndpoint:   otel.AgentEndpoint(serviceName),
		},
	}
}

func (o *OTELNodeDriver) getRemoteWriteEndpoint() string {
	var serviceName string
	service, err := o.getAgentService()
	if err != nil || service == nil {
		serviceName = "opni-agent"
	} else {
		serviceName = service.Name
	}
	return fmt.Sprintf("http://%s.%s.svc/api/agent/push", serviceName, o.namespace)
}

func (o *OTELNodeDriver) getAgentService() (*corev1.Service, error) {
	list := &corev1.ServiceList{}
	if err := o.K8sClient.List(context.TODO(), list,
		client.InNamespace(o.namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	); err != nil {
		return nil, err
	}

	if len(list.Items) != 1 {
		return nil, errors.New("statefulsets found not exactly 1")
	}
	return &list.Items[0], nil
}

func init() {
	drivers.NodeDrivers.Register("opni-manager-otel", func(_ context.Context, opts ...driverutil.Option) (drivers.MetricsNodeDriver, error) {
		options := OTELNodeDriverOptions{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Logger:    logger.NewPluginLogger().Named("metrics").Named("otel"),
		}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}
		return NewOTELDriver(options)
	})
}
