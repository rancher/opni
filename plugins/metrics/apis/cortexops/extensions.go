package cortexops

import (
	"github.com/rancher/opni/internal/codegen/cli"
	pflag "github.com/spf13/pflag"
	"github.com/ttacon/chalk"
)

func (s *InstallStatus) RenderText(out cli.Writer) {
	switch s.State {
	case InstallState_NotInstalled:
		out.Println(chalk.Red.Color("Not Installed"))
	case InstallState_Updating:
		out.Println(chalk.Yellow.Color("Updating"))
	case InstallState_Installed:
		out.Println(chalk.Green.Color("Installed"))
	case InstallState_Uninstalling:
		out.Println(chalk.Yellow.Color("Uninstalling"))
	case InstallState_Unknown:
		out.Println("Unknown")
	}

	out.Printf("Version: %s\n", s.Version)
	for k, v := range s.Metadata {
		out.Printf("%s: %s\n", k, v)
	}
}

func (cc *ClusterConfiguration) LoadDefaults() {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	fs.AddFlagSet(cc.FlagSet())
	if err := fs.Parse([]string{}); err != nil {
		panic(err)
	}
}
