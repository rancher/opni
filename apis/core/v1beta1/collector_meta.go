package v1beta1

func (c *CollectorSpec) IsEmpty() bool {
	return c.LoggingConfig == nil && c.MetricsConfig == nil && c.TracesConfig == nil
}
