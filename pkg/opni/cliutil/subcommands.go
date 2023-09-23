package cliutil

import (
	"strings"

	"github.com/spf13/cobra"
)

func AddSubcommands(cmd *cobra.Command, subcommands ...*cobra.Command) {
	// scan the list of subcommands to see if there are any that share a common
	// space-separated prefix word, which will be interpreted as a subcommand
	// for grouping purposes. For example, with the following `use` values:
	// - "groupname commandname"
	// - "groupname commandname2"
	// - "groupname commandname3"
	// - "notagroupname [--flag]"
	// - "commandname4"
	// the following command tree will be created:
	// - "groupname"
	//   - "commandname"
	//   - "commandname2"
	//   - "commandname3"
	// - "notagroupname"
	// - "commandname4"

	dynamicGroups := make(map[string][]*cobra.Command)
	for _, subcommand := range subcommands {
		use := subcommand.Use
		prefix, _, ok := strings.Cut(use, " ")
		if ok {
			dynamicGroups[prefix] = append(dynamicGroups[prefix], subcommand)
		} else {
			cmd.AddCommand(subcommand)
		}
	}
	for prefix, subcommands := range dynamicGroups {
		if len(subcommands) == 1 {
			cmd.AddCommand(subcommands[0])
			continue
		}

		group := &cobra.Command{
			Use: prefix,
		}
		for _, subcommand := range subcommands {
			_, subcommand.Use, _ = strings.Cut(subcommand.Use, " ")
		}
		AddSubcommands(group, subcommands...)
		cmd.AddCommand(group)
	}
}
