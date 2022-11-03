package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildLoggingUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Logging upgrade commands",
	}

	cmd.AddCommand(BuildOpensearchUpgradeStatusCmd())
	cmd.AddCommand(BuildOpensearchUpgradeDoCmd())

	return cmd
}

func BuildOpensearchUpgradeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Opensearch upgrade status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := loggingAdminClient.UpgradeAvailable(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if status.GetUpgradePending() {
				fmt.Println(chalk.Yellow.Color("Opensearch upgrade is pending"))
				return nil
			}
			fmt.Println(chalk.Green.Color("Opensearch is up to date"))
			return nil
		},
	}
}

func BuildOpensearchUpgradeDoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "do",
		Short: "Initiate the Opensearch upgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := loggingAdminClient.DoUpgrade(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			fmt.Println(chalk.Green.Color("Opensearch upgrade initiated"))
			return nil
		},
	}
}
