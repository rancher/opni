package drivers

import (
	"context"
	"fmt"
	"os"
	"sync"

	"log/slog"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/topology/apis/node"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExternalTopologyOperatorDriver struct {
	ExternalTopologyOperatorDriverOptions
	logger    *slog.Logger
	namespace string

	state reconcilerState
}

type reconcilerState struct {
	sync.Mutex
	running       bool
	backoffCtx    context.Context
	backoffCancel context.CancelFunc
}

type ExternalTopologyOperatorDriverOptions struct {
	k8sClient client.Client
}

type ExternalTopologyOperatorDriverOption func(*ExternalTopologyOperatorDriverOptions)

func (o *ExternalTopologyOperatorDriverOptions) apply(opts ...ExternalTopologyOperatorDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithK8sClient(k8sClient client.Client) ExternalTopologyOperatorDriverOption {
	return func(o *ExternalTopologyOperatorDriverOptions) {
		o.k8sClient = k8sClient
	}
}

func NewExternalTopologyOperatorDriver(
	opts ...ExternalTopologyOperatorDriverOption,
) (*ExternalTopologyOperatorDriver, error) {
	options := ExternalTopologyOperatorDriverOptions{}
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

	return &ExternalTopologyOperatorDriver{
		ExternalTopologyOperatorDriverOptions: options,
		logger:                                logger.New().WithGroup("external-topology-operator-driver"),
		namespace:                             namespace,
	}, nil
}

var _ TopologyNodeDriver = (*ExternalTopologyOperatorDriver)(nil)

func (t *ExternalTopologyOperatorDriver) Name() string {
	return "external-topology-operator"
}

func (t *ExternalTopologyOperatorDriver) ConfigureNode(conf *node.TopologyCapabilityConfig) {
	t.state.Lock()
	if t.state.running {
		t.state.backoffCancel()
	}
	t.state.running = true
	ctx, ca := context.WithCancel(context.TODO())
	t.state.backoffCtx = ctx
	t.state.backoffCancel = ca
	t.state.Unlock()

	// build requirements here

	shouldExist := conf.Enabled
	p := backoff.Exponential()
	b := p.Start(ctx)
	var success bool
BACKOFF:
	for backoff.Continue(b) {
		// iterate over objects owned by this "reconciler" here
		// for _ obj := range X {}
		for _, obj := range []client.Object{} {
			if err := t.reconcileObject(obj, shouldExist); err != nil {
				t.logger.With(
					"object", client.ObjectKeyFromObject(obj).String(),
					logger.Err(err),
				).Error("error reconciling object")
				continue BACKOFF
			}
		}
		success = true
		break
	}
	if !success {
		t.logger.Error("timed out reconciling objects")
	} else {
		t.logger.Info("objects reconciled successfully")
	}
}

func (t *ExternalTopologyOperatorDriver) reconcileObject(_ client.Object, _ bool) error {
	// TODO(topology) : implement me, reconcile objects here
	return nil
}
