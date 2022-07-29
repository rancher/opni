package features

import (
	"context"
	"errors"
	"os"

	"github.com/dbason/featureflags"
	"github.com/rancher/opni/pkg/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/featuregate"
)

const (
	NodeFeatureDiscoveryOperator featuregate.Feature = "NodeFeatureDiscoveryOperator"
	GPUOperator                  featuregate.Feature = "GPUOperator"
)

var (
	DefaultMutableFeatureGate = featuregate.NewFeatureGate()
	featureGates              = map[featuregate.Feature]featuregate.FeatureSpec{
		NodeFeatureDiscoveryOperator: {Default: false, PreRelease: featuregate.Alpha},
		GPUOperator:                  {Default: true, PreRelease: featuregate.Beta},
	}

	ErrUnknownFeatureGate = errors.New("unknown feature gate")

	FeatureList *featureflags.FeatureList
)

func PopulateFeatures(ctx context.Context, config *rest.Config) {
	lg := logger.New().Named("feature-flags")
	ns := os.Getenv("POD_NAMESPACE")

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		lg.Fatalf("failed to create clientset: %s", err)
	}

	FeatureList, err = featureflags.NewFeatureListFromConfigMap(ctx, clientset, ns)
	if err != nil {
		lg.Fatalf("failed to load features: %s", err)
	}

}

func init() {
	DefaultMutableFeatureGate.Add(featureGates)
}
