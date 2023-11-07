package ext

import (
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (c *SampleConfiguration) WithRevision(rev int64) *SampleConfiguration {
	c.Revision = corev1.NewRevision(rev)
	return c
}

func (c *SampleConfiguration) WithoutRevision() *SampleConfiguration {
	c.Revision = nil
	return c
}

// Implements driverutil.ContextKeyable
func (g *SampleGetRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("key")
}

// Implements driverutil.ContextKeyable
func (g *SampleSetRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("key")
}

// Implements driverutil.ContextKeyable
func (g *SampleResetRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("key")
}

// Implements driverutil.ContextKeyable
func (g *SampleHistoryRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("key")
}

// Implements driverutil.ContextKeyable
func (g *SampleDryRunRequest) ContextKey() protoreflect.FieldDescriptor {
	return g.ProtoReflect().Descriptor().Fields().ByName("key")
}
