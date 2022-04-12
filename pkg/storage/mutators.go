package storage

import (
	"github.com/rancher/opni-monitoring/pkg/core"
)

func NewCompositeMutator[T any](mutators ...MutatorFunc[T]) MutatorFunc[T] {
	return func(t T) {
		for _, mutator := range mutators {
			mutator(t)
		}
	}
}

func NewIncrementUsageCountMutator() MutatorFunc[*core.BootstrapToken] {
	return func(obj *core.BootstrapToken) {
		obj.Metadata.UsageCount++
	}
}

func NewAddCapabilityMutator[O core.MetadataAccessor[T], T core.Capability[T]](capability T) MutatorFunc[O] {
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

func NewRemoveCapabilityMutator[O core.MetadataAccessor[T], T core.Capability[T]](capability T) MutatorFunc[O] {
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
