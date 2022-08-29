package commands

import (
	"fmt"

	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildCortexClusterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cortex-cluster",
		Short: "Cortex cluster setup and configuration",
	}
	cmd.AddCommand(BuildCortexClusterStatusCmd())
	cmd.AddCommand(BuildCortexClusterConfigureCmd())
	cmd.AddCommand(BuildCortexClusterUninstallCmd())
	return cmd
}

func BuildCortexClusterStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Cortex cluster status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := opsClient.GetInstallStatus(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			switch status.State {
			case cortexops.InstallState_NotInstalled:
				fmt.Println(chalk.Red.Color("Not installed"))
				return nil
			case cortexops.InstallState_Updating:
				fmt.Println(chalk.Yellow.Color("Updating"))
			case cortexops.InstallState_Installed:
				fmt.Println(chalk.Green.Color("Installed"))
			case cortexops.InstallState_Uninstalling:
				fmt.Println(chalk.Yellow.Color("Uninstalling"))
				return nil
			case cortexops.InstallState_Unknown:
				fmt.Println("Unknown")
				return nil
			}

			fmt.Printf("Version: %s\n", status.Version)
			for k, v := range status.Metadata {
				fmt.Printf("%s: %s\n", k, v)
			}
			return nil
		},
	}
	return cmd
}

func BuildCortexClusterConfigureCmd() *cobra.Command {
	var mode string
	var installConf cortexops.InstallConfiguration
	var storage storagev1.StorageSpec
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Install or configure a Cortex cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			strategy, ok := cortexops.DeploymentMode_value[mode]
			if !ok {
				return fmt.Errorf("unknown deployment strategy %s", mode)
			}
			installConf.Mode = cortexops.DeploymentMode(strategy)
			installConf.Storage = &storage

			_, err := opsClient.ConfigureInstall(cmd.Context(), &installConf)
			return err
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Deployment mode (one of: AllInOne, HighlyAvailable)")
	cmd.Flags().AddFlagSet(storage.FlagSet())
	return cmd
}

func BuildCortexClusterUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall a Cortex cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := opsClient.UninstallCluster(cmd.Context(), &emptypb.Empty{})
			return err
		},
	}
	return cmd
}
