package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/apis"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/otel"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/k8sutil/provider"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/apis/node"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	secretName         = "opni-internal-auth"
	secretKey          = "password"
	dataPrepperName    = "opni-shipper"
	dataPrepperVersion = "2.6.0"
	clusterOutputName  = "opni-output"
	clusterFlowName    = "opni-flow"
	loggingConfigName  = "opni-logging"
	traceConfigName    = "opni-tracing"
)

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions
	provider  string
	k8sClient client.Client

	state reconcilerState
}

type reconcilerState struct {
	sync.Mutex
	running       bool
	backoffCtx    context.Context
	backoffCancel context.CancelFunc
}

type KubernetesManagerDriverOptions struct {
	Namespace  string       `option:"namespace"`
	RestConfig *rest.Config `option:"restConfig"`
	Logger     *slog.Logger `option:"logger"`
}

func NewKubernetesManagerDriver(options KubernetesManagerDriverOptions) (*KubernetesManagerDriver, error) {
	scheme := apis.NewScheme()
	if options.RestConfig == nil {
		var err error
		options.RestConfig, err = k8sutil.NewRestConfig(k8sutil.ClientOptions{
			Scheme: scheme,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(options.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	provider, err := provider.DetectProvider(context.TODO(), clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	k8sClient, err := client.New(options.RestConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &KubernetesManagerDriver{
		KubernetesManagerDriverOptions: options,
		k8sClient:                      k8sClient,
		provider:                       provider,
	}, nil
}

var _ drivers.LoggingNodeDriver = (*KubernetesManagerDriver)(nil)

func (m *KubernetesManagerDriver) ConfigureNode(config *node.LoggingCapabilityConfig) {
	m.state.Lock()
	if m.state.running {
		m.state.backoffCancel()
	}
	m.state.running = true
	ctx, ca := context.WithCancel(context.TODO())
	m.state.backoffCtx = ctx
	m.state.backoffCancel = ca
	m.state.Unlock()

	p := backoff.Exponential()
	b := p.Start(ctx)
	var success bool
BACKOFF:
	for backoff.Continue(b) {
		logCollectorConf := m.buildLoggingCollectorConfig()
		if err := m.reconcileObject(logCollectorConf, config.Enabled); err != nil {
			m.Logger.With(
				"object", client.ObjectKeyFromObject(logCollectorConf).String(),
				logger.Err(err),
			).Error("error reconciling object")
			continue BACKOFF
		}

		if err := m.reconcileCollector(config.Enabled); err != nil {
			m.Logger.With(
				"object", "opni collector",
				logger.Err(err),
			).Error("error reconciling object")
		}

		if err := m.reconcileDataPrepper(config.Enabled); err != nil {
			m.Logger.With(
				"object", "opni data prepper",
				logger.Err(err),
			).Error("error reconciling object")
		}

		success = true
		break
	}

	if !success {
		m.Logger.Error("timed out reconciling objects")
	} else {
		m.Logger.Info("objects reconciled successfully")
	}
}

//
//func (m *KubernetesManagerDriver) buildAuthSecret(config *node.OpensearchConfig) *corev1.Secret {
//	secret := &corev1.Secret{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      secretName,
//			Namespace: m.Namespace,
//		},
//	}
//	if config != nil {
//		secret.StringData = map[string]string{
//			secretKey: config.Password,
//		}
//	}
//	return secret
//}

func (m *KubernetesManagerDriver) buildDataPrepper() *opniloggingv1beta1.DataPrepper {
	dataPrepper := &opniloggingv1beta1.DataPrepper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataPrepperName,
			Namespace: m.Namespace,
		},
		Spec: opniloggingv1beta1.DataPrepperSpec{
			//Username: config.Username,
			PasswordFrom: &corev1.SecretKeySelector{
				Key: secretKey,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
			//Opensearch: &opniloggingv1beta1.OpensearchSpec{
			//	InsecureDisableSSLVerify: false,
			//},
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      "opni", // TODO: update
				Namespace: m.Namespace,
			},
			//ClusterID:     m.clusterID,
			EnableTracing: true,
			Version:       dataPrepperVersion,
		},
	}
	return dataPrepper
}

func (m *KubernetesManagerDriver) buildLoggingCollectorConfig() *opniloggingv1beta1.CollectorConfig {
	collectorConfig := &opniloggingv1beta1.CollectorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-logging",
		},
		Spec: opniloggingv1beta1.CollectorConfigSpec{
			Provider: opniloggingv1beta1.LogProvider(m.provider),
			KubeAuditLogs: &opniloggingv1beta1.KubeAuditLogsSpec{
				Enabled: false,
			},
		},
	}
	return collectorConfig
}

// TODO: make this generic
func (m *KubernetesManagerDriver) reconcileObject(desired client.Object, shouldExist bool) error {
	// get the object
	key := client.ObjectKeyFromObject(desired)
	lg := m.Logger.With("object", key)
	lg.Info("reconciling object")

	// get the agent statefulset
	agentStatefulSet, err := m.getAgentSet()
	if err != nil {
		return err
	}

	current := desired.DeepCopyObject().(client.Object)
	err = m.k8sClient.Get(context.TODO(), key, current)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	// this can error if the object is cluster-scoped, but that's ok
	controllerutil.SetOwnerReference(agentStatefulSet, desired, m.k8sClient.Scheme())

	if k8serrors.IsNotFound(err) {
		if !shouldExist {
			lg.Info("object does not exist and should not exist, skipping")
			return nil
		}
		lg.Info("object does not exist, creating")
		// create the object
		return m.k8sClient.Create(context.TODO(), desired)
	} else if !shouldExist {
		// delete the object
		lg.Info("object exists and should not exist, deleting")
		return m.k8sClient.Delete(context.TODO(), current)
	}

	// update the object
	return m.patchObject(current, desired)
}

func (m *KubernetesManagerDriver) reconcileCollector(shouldExist bool) error {
	coll := &opnicorev1beta1.Collector{
		ObjectMeta: metav1.ObjectMeta{
			Name: otel.CollectorName,
		},
	}
	err := m.k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(coll), coll)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	if k8serrors.IsNotFound(err) && shouldExist {
		coll = m.buildEmptyCollector()
		coll.Spec.LoggingConfig = &corev1.LocalObjectReference{
			Name: loggingConfigName,
		}
		coll.Spec.TracesConfig = &corev1.LocalObjectReference{
			Name: traceConfigName,
		}

		return m.k8sClient.Create(context.TODO(), coll)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := m.k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(coll), coll)
		if err != nil {
			return err
		}
		if shouldExist {
			coll.Spec.LoggingConfig = &corev1.LocalObjectReference{
				Name: loggingConfigName,
			}
			coll.Spec.TracesConfig = &corev1.LocalObjectReference{
				Name: traceConfigName,
			}
		} else {
			coll.Spec.LoggingConfig = nil
			coll.Spec.TracesConfig = nil
		}

		return m.k8sClient.Update(context.TODO(), coll)
	})
	return err
}

func (m *KubernetesManagerDriver) reconcileDataPrepper(shouldExist bool) error {
	dataPrepper := &opniloggingv1beta1.DataPrepper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataPrepperName,
			Namespace: m.Namespace,
		},
	}
	err := m.k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dataPrepper), dataPrepper)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	if k8serrors.IsNotFound(err) && shouldExist {
		dataPrepper = m.buildDataPrepper()
		return m.k8sClient.Create(context.TODO(), dataPrepper)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := m.k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(dataPrepper), dataPrepper)
		if err != nil {
			return err
		}
		return m.k8sClient.Update(context.TODO(), dataPrepper)
	})
	return err
}

func (m *KubernetesManagerDriver) getAgentSet() (*appsv1.Deployment, error) {
	list := &appsv1.DeploymentList{}
	if err := m.k8sClient.List(context.TODO(), list,
		client.InNamespace(m.Namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	); err != nil {
		return nil, err
	}

	if len(list.Items) != 1 {
		return nil, errors.New("deployments found not exactly 1")
	}
	return &list.Items[0], nil
}

func (m *KubernetesManagerDriver) getAgentService() (*corev1.Service, error) {
	list := &corev1.ServiceList{}
	if err := m.k8sClient.List(context.TODO(), list,
		client.InNamespace(m.Namespace),
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

func (m *KubernetesManagerDriver) patchObject(current client.Object, desired client.Object) error {
	// update the object
	patchResult, err := patch.DefaultPatchMaker.Calculate(current, desired, patch.IgnoreStatusFields())
	if err != nil {
		m.Logger.With(
			logger.Err(err),
		).Warn("could not match objects")
		return err
	}
	if patchResult.IsEmpty() {
		m.Logger.Info("resource is in sync")
		return nil
	}
	m.Logger.Info("resource diff")

	if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(desired); err != nil {
		m.Logger.With(
			logger.Err(err),
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

	m.Logger.Info("updating resource")

	return m.k8sClient.Update(context.TODO(), desired)
}

func (m *KubernetesManagerDriver) buildEmptyCollector() *opnicorev1beta1.Collector {
	var serviceName string
	service, err := m.getAgentService()
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
			SystemNamespace:          m.Namespace,
			AgentEndpoint:            otel.AgentEndpoint(serviceName),
			AggregatorOTELConfigSpec: opnicorev1beta1.NewDefaultAggregatorOTELConfigSpec(),
			NodeOTELConfigSpec:       opnicorev1beta1.NewDefaultNodeOTELConfigSpec(),
		},
	}
}

func init() {
	drivers.NodeDrivers.Register("kubernetes-manager", func(_ context.Context, opts ...driverutil.Option) (drivers.LoggingNodeDriver, error) {
		options := KubernetesManagerDriverOptions{
			Namespace: os.Getenv("POD_NAMESPACE"),
			Logger:    logger.NewPluginLogger().WithGroup("logging").WithGroup("kubernetes-manager"),
		}
		driverutil.ApplyOptions(&options, opts...)
		return NewKubernetesManagerDriver(options)
	})
}
