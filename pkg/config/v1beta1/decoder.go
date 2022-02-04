package v1beta1

import (
	"fmt"

	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"sigs.k8s.io/yaml"
)

func DecodeObject(kind string, document []byte) (meta.Object, error) {
	switch kind {
	case "GatewayConfig":
		obj := &GatewayConfig{}
		if err := yaml.UnmarshalStrict(document, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case "AuthProvider":
		obj := &AuthProvider{}
		if err := yaml.UnmarshalStrict(document, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case "AgentConfig":
		obj := &AgentConfig{}
		if err := yaml.UnmarshalStrict(document, obj); err != nil {
			return nil, err
		}
		return obj, nil
	}
	return nil, fmt.Errorf("%w: %s", meta.ErrUnknownObjectKind, kind)
}
