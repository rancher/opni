package commands

import (
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	OpniComponents = &templates.CommandGroup{
		Message: "Opni Components:",
	}
	ManagementAPI = &templates.CommandGroup{
		Message: "Management API:",
	}
	PluginAPIs = &templates.CommandGroup{
		Message: "Plugin APIs:",
	}
	Utilities = &templates.CommandGroup{
		Message: "Utilities:",
	}
	Debug = &templates.CommandGroup{
		Message: "Debug:",
	}
)

func AddCommandsToGroup(group *templates.CommandGroup, cmds ...*cobra.Command) {
	group.Commands = append(group.Commands, cmds...)
}
