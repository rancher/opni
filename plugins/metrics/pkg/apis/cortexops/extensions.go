package cortexops

import (
	"github.com/rancher/opni/internal/cli"
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
