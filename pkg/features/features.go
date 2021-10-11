package features

import (
	"errors"

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
)

func init() {
	DefaultMutableFeatureGate.Add(featureGates)
}
