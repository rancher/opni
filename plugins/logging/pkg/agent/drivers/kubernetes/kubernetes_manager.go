package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/apis"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/util/k8sutil/provider"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	secretName        = "opni-opensearch-auth"
	secretKey         = "password"
	dataPrepperName   = "opni-shipper"
	clusterOutputName = "opni-output"
	clusterFlowName   = "opni-flow"
)

type KubernetesManagerDriver struct {
	KubernetesManagerDriverOptions
	logger    *zap.SugaredLogger
	namespace string
	k8sClient client.Client
	clusterID string
	provider  string

	state reconcilerState
}

type reconcilerState struct {
	sync.Mutex
	running       bool
	backoffCtx    context.Context
	backoffCancel context.CancelFunc
}

type KubernetesManagerDriverOptions struct {
	restConfig *rest.Config
}

type KubernetesManagerDriverOption func(*KubernetesManagerDriverOptions)

func (o *KubernetesManagerDriverOptions) apply(opts ...KubernetesManagerDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithRestConfig(restConfig *rest.Config) KubernetesManagerDriverOption {
	return func(o *KubernetesManagerDriverOptions) {
		o.restConfig = restConfig
	}
}

func NewKubernetesManagerDriver(
	logger *zap.SugaredLogger,
	opts ...KubernetesManagerDriverOption,
) (*KubernetesManagerDriver, error) {
	options := KubernetesManagerDriverOptions{}
	options.apply(opts...)

	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return nil, fmt.Errorf("POD_NAMESPACE environment variable not set")
	}

	if options.restConfig == nil {
		restConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create rest config: %w", err)
		}
		options.restConfig = restConfig
	}

	k8sClient, err := client.New(options.restConfig, client.Options{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8sClient: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(options.restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	clusterID, err := getClusterId(context.TODO(), k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cluster ID: %w", err)
	}

	provider, err := provider.DetectProvider(context.TODO(), clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	return &KubernetesManagerDriver{
		KubernetesManagerDriverOptions: options,
		logger:                         logger,
		namespace:                      namespace,
		clusterID:                      clusterID,
		provider:                       provider,
	}, nil

}

func (m *KubernetesManagerDriver) Name() string {
	return "kubernetes-manager"
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
		for _, obj := range []client.Object{
			m.buildAuthSecret(config.GetOpensearchConfig()),
			m.buildDataPrepper(config.GetOpensearchConfig()),
			m.buildOpniClusterOutput(),
			m.buildOpniClusterFlow(),
			m.buildLogAdapter(),
		} {
			if err := m.reconcileObject(obj, config.Enabled); err != nil {
				m.logger.With(
					"object", client.ObjectKeyFromObject(obj).String(),
					zap.Error(err),
				).Error("error reconciling object")
				continue BACKOFF
			}
		}
		success = true
		break
	}

	if !success {
		m.logger.Error("timed out reconciling objects")
	} else {
		m.logger.Info("objects reconciled successfully")
	}
}

func (m *KubernetesManagerDriver) buildAuthSecret(config *node.OpensearchConfig) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: m.namespace,
		},
		StringData: map[string]string{
			secretKey: config.Password,
		},
	}
}

func (m *KubernetesManagerDriver) buildDataPrepper(config *node.OpensearchConfig) *opniloggingv1beta1.DataPrepper {
	return &opniloggingv1beta1.DataPrepper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataPrepperName,
			Namespace: m.namespace,
		},
		Spec: opniloggingv1beta1.DataPrepperSpec{
			Username: config.Username,
			PasswordFrom: &corev1.SecretKeySelector{
				Key: secretKey,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
			Opensearch: &opniloggingv1beta1.OpensearchSpec{
				Endpoint:                 config.Url,
				InsecureDisableSSLVerify: false,
			},
			ClusterID:     m.clusterID,
			EnableTracing: true,
		},
	}
}

func (m *KubernetesManagerDriver) buildOpniClusterOutput() *loggingv1beta1.ClusterOutput {
	return &loggingv1beta1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterOutputName,
			Namespace: m.namespace,
		},
		Spec: loggingv1beta1.ClusterOutputSpec{
			OutputSpec: loggingv1beta1.OutputSpec{
				HTTPOutput: &output.HTTPOutputConfig{
					Endpoint:    fmt.Sprintf("http://%s.%s:2021/log/ingest", dataPrepperName, m.namespace),
					ContentType: "application/json",
					JsonArray:   true,
					Buffer: &output.Buffer{
						Tags:           "[]",
						FlushInterval:  "2s",
						ChunkLimitSize: "1mb",
					},
				},
			},
		},
	}
}

func (m *KubernetesManagerDriver) buildOpniClusterFlow() *loggingv1beta1.ClusterFlow {
	return &loggingv1beta1.ClusterFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterFlowName,
			Namespace: m.namespace,
		},
		Spec: loggingv1beta1.ClusterFlowSpec{
			Filters: []loggingv1beta1.Filter{
				{
					Dedot: &filter.DedotFilterConfig{
						Separator: "-",
						Nested:    true,
					},
				},
				{
					Grep: &filter.GrepConfig{
						Exclude: []filter.ExcludeSection{
							{
								Key:     "log",
								Pattern: `^\n$`,
							},
						},
					},
				},
				{
					DetectExceptions: &filter.DetectExceptions{
						Languages: []string{
							"java",
							"python",
							"go",
							"ruby",
							"js",
							"csharp",
							"php",
						},
						MultilineFlushInterval: "0.1",
					},
				},
				{
					RecordTransformer: &filter.RecordTransformer{
						Records: []filter.Record{
							{
								"cluster_id": m.clusterID,
							},
						},
					},
				},
			},
			Match: []loggingv1beta1.ClusterMatch{
				{
					ClusterExclude: &loggingv1beta1.ClusterExclude{
						Namespaces: []string{
							"opni-system",
						},
					},
				},
				{
					ClusterSelect: &loggingv1beta1.ClusterSelect{},
				},
			},
			GlobalOutputRefs: []string{
				clusterOutputName,
			},
		},
	}
}

func (m *KubernetesManagerDriver) buildLogAdapter() *opniloggingv1beta1.LogAdapter {
	return &opniloggingv1beta1.LogAdapter{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opni-logging",
		},
		Spec: opniloggingv1beta1.LogAdapterSpec{
			Provider:         opniloggingv1beta1.LogProvider(m.provider),
			ControlNamespace: &m.namespace,
		},
	}
}

// TODO: make this generic
func (m *KubernetesManagerDriver) reconcileObject(desired client.Object, shouldExist bool) error {
	// get the object
	key := client.ObjectKeyFromObject(desired)
	lg := m.logger.With("object", key)
	lg.Info("reconciling object")

	// get the agent statefulset
	list := &appsv1.StatefulSetList{}
	if err := m.k8sClient.List(context.TODO(), list,
		client.InNamespace(m.namespace),
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
	err := m.k8sClient.Get(context.TODO(), key, current)
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
	patchResult, err := patch.DefaultPatchMaker.Calculate(current, desired, patch.IgnoreStatusFields())
	if err != nil {
		m.logger.With(
			zap.Error(err),
		).Warn("could not match objects")
		return err
	}
	if patchResult.IsEmpty() {
		m.logger.Info("resource is in sync")
		return nil
	}
	m.logger.Info("resource diff")

	if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(desired); err != nil {
		m.logger.With(
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

	m.logger.Info("updating resource")

	return m.k8sClient.Update(context.TODO(), desired)
}

func getClusterId(ctx context.Context, k8sClient client.Client) (string, error) {
	systemNamespace := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name: "kube-system",
	}, systemNamespace); err != nil {
		return "", err
	}

	return string(systemNamespace.GetUID()), nil
}
