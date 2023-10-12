package node

func (*MetricsCapabilityConfig) RedactSecrets()                                 {}
func (*MetricsCapabilityConfig) UnredactSecrets(*MetricsCapabilityConfig) error { return nil }
