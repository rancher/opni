package storage

import (
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func NewCompositeMutator[T any](mutators ...MutatorFunc[T]) MutatorFunc[T] {
	return func(t T) {
		for _, mutator := range mutators {
			mutator(t)
		}
	}
}

func NewIncrementUsageCountMutator() MutatorFunc[*corev1.BootstrapToken] {
	return func(obj *corev1.BootstrapToken) {
		obj.Metadata.UsageCount++
	}
}

func NewAddCapabilityMutator[O corev1.MetadataAccessor[T], T corev1.Capability[T]](capability T) MutatorFunc[O] {
	return func(obj O) {
		exists := false
		for _, c := range obj.GetCapabilities() {
			if c.Equal(capability) {
				exists = true
				break
			}
		}
		if !exists {
			obj.SetCapabilities(
				append(obj.GetCapabilities(), capability))
		}
	}
}

func NewRemoveCapabilityMutator[O corev1.MetadataAccessor[T], T corev1.Capability[T]](capability T) MutatorFunc[O] {
	return func(obj O) {
		capabilities := []T{}
		for _, c := range obj.GetCapabilities() {
			if !c.Equal(capability) {
				capabilities = append(capabilities, c)
			}
		}
		obj.SetCapabilities(capabilities)
	}
}
