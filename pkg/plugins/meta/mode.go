package meta

type PluginMode string

const (
	ModeUnknown PluginMode = ""
	ModeGateway PluginMode = "gateway"
	ModeAgent   PluginMode = "agent"
)
