package features

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Patches the given workload object to append the '--feature-gates=...' argument
// to the container with name containerName.
// The object must have the unstructured path 'spec/template/spec/containers/<containerName>/args'.
func PatchFeatureGatesInWorkload(obj *unstructured.Unstructured, containerName string) error {
	// Find the container index
	var container map[string]interface{}
	var containerIndex int
	containers, _, err := unstructured.NestedSlice(obj.Object,
		"spec", "template", "spec", "containers")
	if err != nil {
		return fmt.Errorf("failed to get containers from workload: %w", err)
	}
	for i, c := range containers {
		name, _, err := unstructured.NestedString(c.(map[string]interface{}), "name")
		if err != nil {
			return fmt.Errorf("failed to get container name: %w", err)
		}
		if name == containerName {
			container = c.(map[string]interface{})
			containerIndex = i
			break
		}
	}
	if container == nil {
		return fmt.Errorf("failed to find container with name %s", containerName)
	}
	argList, _, err := unstructured.NestedStringSlice(container, "args")
	if err != nil {
		return fmt.Errorf("failed to list args of container %s: %w", containerName, err)
	}
	featureGateArg := DefaultMutableFeatureGate.String()
	if len(featureGateArg) > 0 {
		argList = append(argList, fmt.Sprintf("--feature-gates=%q", featureGateArg))
	}
	if err := unstructured.SetNestedStringSlice(container, argList, "args"); err != nil {
		return fmt.Errorf("failed to set args of container %s: %w", containerName, err)
	}
	containers[containerIndex] = container
	if err := unstructured.SetNestedSlice(obj.Object, containers,
		"spec", "template", "spec", "containers"); err != nil {
		return fmt.Errorf("failed to set containers in workload: %w", err)
	}
	return nil
}
