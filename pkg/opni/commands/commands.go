package commands

import (
	"github.com/spf13/cobra"
)

var (
	OpniComponents = &cobra.Group{
		ID:    "opni-components",
		Title: "Opni Components:",
	}
	ManagementAPI = &cobra.Group{
		ID:    "management-api",
		Title: "Management API:",
	}
	PluginAPIs = &cobra.Group{
		ID:    "plugin-apis",
		Title: "Plugin APIs:",
	}
	Utilities = &cobra.Group{
		ID:    "utilities",
		Title: "Utilities:",
	}
	Debug = &cobra.Group{
		ID:    "debug",
		Title: "Debug:",
	}
	AllGroups = []*cobra.Group{
		OpniComponents,
		ManagementAPI,
		PluginAPIs,
		Utilities,
		Debug,
	}
)

var AllCommands []*cobra.Command

func AddCommandsToGroup(group *cobra.Group, cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		cmd.GroupID = group.ID
	}
	AllCommands = append(AllCommands, cmds...)
}
