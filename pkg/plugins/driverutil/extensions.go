package driverutil

import (
	"github.com/rancher/opni/internal/codegen/cli"
	"github.com/ttacon/chalk"
)

func (s *InstallStatus) RenderText(out cli.Writer) {
	out.Print("Configuration State: ")
	switch s.ConfigState {
	case ConfigurationState_NotConfigured:
		out.Println(chalk.Dim.TextStyle("Not Configured"))
	case ConfigurationState_Configured:
		out.Println(chalk.Green.Color("Configured"))
	}

	out.Print("Install State: ")
	switch s.InstallState {
	case InstallState_NotInstalled:
		out.Println(chalk.Dim.TextStyle("Not Installed"))
	case InstallState_Installed:
		out.Println(chalk.Green.Color("Installed"))
	}

	out.Print("Application State: ")
	switch s.AppState {
	case ApplicationState_NotRunning:
		out.Println(chalk.Dim.TextStyle("Not Running"))
	case ApplicationState_Pending:
		out.Println(chalk.Yellow.Color("Pending"))
	case ApplicationState_Running:
		out.Println(chalk.Green.Color("Running"))
	case ApplicationState_Failed:
		out.Println(chalk.Red.Color("Failed"))
	}
	if len(s.GetWarnings()) > 0 {
		out.Println(chalk.Yellow.Color("Warnings:"))
		for _, warning := range s.GetWarnings() {
			out.Printf(" - %s\n", warning)
		}
	}

	out.Printf("Version: %s\n", s.Version)
	for k, v := range s.Metadata {
		out.Printf("%s: %s\n", k, v)
	}
}
