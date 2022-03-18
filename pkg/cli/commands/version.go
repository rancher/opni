package commands

import (
	"fmt"
	"runtime/debug"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

func BuildVersionCmd() *cobra.Command {
	var quiet bool
	var verbose bool
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show opni-monitoring version information",
		Run: func(cmd *cobra.Command, args []string) {
			info, _ := debug.ReadBuildInfo()
			if verbose {
				fmt.Println(info.String())
				return
			}
			settings := lo.KeyBy(info.Settings, func(v debug.BuildSetting) string {
				return v.Key
			})
			var noun string
			var version string
			if info.Main.Version == "(devel)" {
				noun = "revision"
				version = settings["vcs.revision"].Value
			} else {
				noun = "version"
				version = info.Main.Version
			}
			if quiet {
				fmt.Println(version)
				return
			}
			fmt.Printf("Opni Monitoring, %s %s\n", noun, version)
			fmt.Printf("  go version: %s\n", info.GoVersion)
			fmt.Printf("  build date: %s\n", settings["vcs.time"].Value)
		},
	}
	versionCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only print version or revision")
	versionCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show verbose version and dependency information")
	return versionCmd
}
