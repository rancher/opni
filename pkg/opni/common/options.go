package common

// These constants are available to all opnictl sub-commands and are filled
// in by the root command using persistent flags.

var (
	NamespaceFlagValue string
	DisableUsage       bool
)

const (
	DefaultOpniNamespace = "opni-system"
)
