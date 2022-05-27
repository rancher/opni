package v1

type Capability[T any] interface {
	Comparator[T]
}

type MetadataAccessor[T Capability[T]] interface {
	GetCapabilities() []T
	SetCapabilities(capabilities []T)
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
}

func (t *BootstrapToken) GetCapabilities() []*TokenCapability {
	return t.GetMetadata().GetCapabilities()
}

func (c *Cluster) GetCapabilities() []*ClusterCapability {
	return c.GetMetadata().GetCapabilities()
}

func (t *BootstrapToken) GetLabels() map[string]string {
	return t.GetMetadata().GetLabels()
}

func (c *Cluster) GetLabels() map[string]string {
	return c.GetMetadata().GetLabels()
}

func (t *BootstrapToken) SetCapabilities(capabilities []*TokenCapability) {
	if t.Metadata == nil {
		t.Metadata = &BootstrapTokenMetadata{}
	}
	t.Metadata.Capabilities = capabilities
}

func (c *Cluster) SetCapabilities(capabilities []*ClusterCapability) {
	if c.Metadata == nil {
		c.Metadata = &ClusterMetadata{}
	}
	c.Metadata.Capabilities = capabilities
}

func (t *BootstrapToken) SetLabels(labels map[string]string) {
	if t.Metadata == nil {
		t.Metadata = &BootstrapTokenMetadata{}
	}
	t.Metadata.Labels = labels
}

func (c *Cluster) SetLabels(labels map[string]string) {
	if c.Metadata == nil {
		c.Metadata = &ClusterMetadata{}
	}
	c.Metadata.Labels = labels
}
