package drivers

import (
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type EmbeddedManagerNodeDriver struct {
	logger  *zap.SugaredLogger
	manager ctrl.Manager
}

type EmbeddedManagerNodeDriverOptions struct {
	restConfig *rest.Config
}

type EmbeddedManagerNodeDriverOption func(*EmbeddedManagerNodeDriverOptions)

func (o *EmbeddedManagerNodeDriverOptions) apply(opts ...EmbeddedManagerNodeDriverOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithRestConfig(restConfig *rest.Config) EmbeddedManagerNodeDriverOption {
	return func(o *EmbeddedManagerNodeDriverOptions) {
		o.restConfig = restConfig
	}
}

func NewEmbeddedManagerNodeDriver(
	logger *zap.SugaredLogger,
	opts ...EmbeddedManagerNodeDriverOption,
) (*EmbeddedManagerNodeDriver, error) {
	ctrl.SetLogger(ctrlzap.New(
		ctrlzap.Level(logger.Desugar().Core()),
		ctrlzap.Encoder(zapcore.NewConsoleEncoder(testutil.EncoderConfig)),
	))

	options := EmbeddedManagerNodeDriverOptions{}
	options.apply(opts...)

	if options.restConfig == nil {
		var err error
		options.restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	mgr, err := ctrl.NewManager(options.restConfig, ctrl.Options{
		Scheme:             apis.NewScheme(),
		MetricsBindAddress: "0",
		LeaderElection:     false,
	})
	if err != nil {
		return nil, err
	}

	return &EmbeddedManagerNodeDriver{
		manager: mgr,
	}, nil
}

var _ MetricsNodeDriver = (*EmbeddedManagerNodeDriver)(nil)

func (d *EmbeddedManagerNodeDriver) Name() string {
	return "embedded-manager"
}

func (d *EmbeddedManagerNodeDriver) ConfigureNode(_ *node.MetricsCapabilityConfig) {

}
