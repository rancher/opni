package v1

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type Capability[T any] interface {
	Comparator[T]
	fmt.Stringer
}

type MetadataAccessor[T Capability[T]] interface {
	GetCapabilities() []T
	SetCapabilities(capabilities []T)
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
	GetResourceVersion() string
	SetResourceVersion(version string)
}

func (t *BootstrapToken) GetCapabilities() []*TokenCapability {
	return t.GetMetadata().GetCapabilities()
}

func (t *BootstrapToken) SetCapabilities(capabilities []*TokenCapability) {
	if t.Metadata == nil {
		t.Metadata = &BootstrapTokenMetadata{}
	}
	t.Metadata.Capabilities = capabilities
}

func (t *BootstrapToken) GetLabels() map[string]string {
	return t.GetMetadata().GetLabels()
}

func (t *BootstrapToken) SetLabels(labels map[string]string) {
	if t.Metadata == nil {
		t.Metadata = &BootstrapTokenMetadata{}
	}
	t.Metadata.Labels = labels
}

func (t *BootstrapToken) GetResourceVersion() string {
	return t.GetMetadata().GetResourceVersion()
}

func (t *BootstrapToken) SetResourceVersion(version string) {
	if t.Metadata == nil {
		t.Metadata = &BootstrapTokenMetadata{}
	}
	t.Metadata.ResourceVersion = version
}

func (c *Cluster) GetCapabilities() []*ClusterCapability {
	return c.GetMetadata().GetCapabilities()
}

func (c *Cluster) SetCapabilities(capabilities []*ClusterCapability) {
	if c.Metadata == nil {
		c.Metadata = &ClusterMetadata{}
	}
	c.Metadata.Capabilities = capabilities
}

func (c *Cluster) GetLabels() map[string]string {
	return c.GetMetadata().GetLabels()
}

func (c *Cluster) SetLabels(labels map[string]string) {
	if c.Metadata == nil {
		c.Metadata = &ClusterMetadata{}
	}
	c.Metadata.Labels = labels
}

func (c *Cluster) GetResourceVersion() string {
	return c.GetMetadata().GetResourceVersion()
}

func (c *Cluster) SetResourceVersion(version string) {
	if c.Metadata == nil {
		c.Metadata = &ClusterMetadata{}
	}
	c.Metadata.ResourceVersion = version
}

func (c *Cluster) SetCreationTimestamp(ts time.Time) {
	if c.Metadata == nil {
		c.Metadata = &ClusterMetadata{}
	}
	c.Metadata.CreationTimestamp = timestamppb.New(ts)
}

func (c *Cluster) GetCreationTimestamp() time.Time {
	return c.GetMetadata().GetCreationTimestamp().AsTime()
}

type lastKnownConnectionDetailsKeyType struct{}

var lastKnownConnectionDetailsKey lastKnownConnectionDetailsKeyType

func LastKnownConnectionDetailsFromContext(ctx context.Context) (*LastKnownConnectionDetails, bool) {
	v := ctx.Value(lastKnownConnectionDetailsKey)
	if v == nil {
		return nil, false
	}
	details, ok := v.(*LastKnownConnectionDetails)
	return details, ok
}

func NewContextWithLastKnownConnectionDetails(ctx context.Context, details *LastKnownConnectionDetails) context.Context {
	return context.WithValue(ctx, lastKnownConnectionDetailsKey, details)
}
