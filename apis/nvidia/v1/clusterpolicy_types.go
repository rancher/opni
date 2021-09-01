package v1

import (
	nvidiav1 "github.com/NVIDIA/gpu-operator/api/v1"
)

func init() {
	SchemeBuilder.Register(
		&nvidiav1.ClusterPolicy{}, &nvidiav1.ClusterPolicyList{},
	)
}
