package v1beta1

func (c *CollectorSpec) IsEmpty() bool {
	empty := true
	if c.LoggingConfig != nil {
		empty = false
	}

	return empty
}
