package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/management"
	cliutil "github.com/rancher/opni/pkg/opnim/util"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/etcdctl/v3/ctlv3"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/yaml"
)

func BuildDebugCmd() *cobra.Command {
	debugCmd := &cobra.Command{
		Use:   "debug",
		Short: "Various debugging commands",
	}
	debugCmd.AddCommand(BuildDebugReloadCmd())
	debugCmd.AddCommand(BuildDebugGetConfigCmd())
	debugCmd.AddCommand(BuildDebugEtcdctlCmd())
	ConfigureManagementCommand(debugCmd)
	return debugCmd
}

func BuildDebugGetConfigCmd() *cobra.Command {
	var format string
	debugGetConfigCmd := &cobra.Command{
		Use:   "get-config",
		Short: "Print the current gateway config to stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := client.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if len(config.Documents) == 0 {
				lg.Warn("server returned no configuration")
				return nil
			}
			for _, doc := range config.Documents {
				switch format {
				case "json":
					buf := new(bytes.Buffer)
					err := json.Indent(buf, doc.Json, "", "  ")
					if err != nil {
						return err
					}
					fmt.Println(buf.String())
				case "yaml":
					data, err := yaml.JSONToYAML(doc.Json)
					if err != nil {
						return err
					}
					fmt.Println(string(data))
				default:
					return fmt.Errorf("unknown format: %s", format)
				}
			}
			return nil
		},
	}
	debugGetConfigCmd.Flags().StringVarP(&format, "format", "f", "yaml", "Output format (yaml|json)")
	return debugGetConfigCmd
}

func BuildDebugReloadCmd() *cobra.Command {
	debugReloadCmd := &cobra.Command{
		Use:   "reload",
		Short: "Signal the gateway to reload and apply any config updates",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := client.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			docsNoSchema := []*management.ConfigDocument{}
			for _, doc := range config.Documents {
				docsNoSchema = append(docsNoSchema, &management.ConfigDocument{
					Json: doc.Json,
				})
			}
			_, err = client.UpdateConfig(cmd.Context(), &management.UpdateConfigRequest{
				Documents: docsNoSchema,
			})
			return err
		},
	}
	return debugReloadCmd
}

func BuildDebugEtcdctlCmd() *cobra.Command {
	debugEtcdctlCmd := &cobra.Command{
		Use:                "etcdctl",
		Short:              "embedded auto-configured etcdctl",
		Long:               "To specify a config location, use the OPNIM_CONFIG environment variable.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			objects := cliutil.LoadConfigObjectsOrDie(os.Getenv("OPNIM_CONFIG"), lg)
			var gatewayConfig *v1beta1.GatewayConfig
			objects.Visit(
				func(config *v1beta1.GatewayConfig) {
					if gatewayConfig == nil {
						gatewayConfig = config
					}
				},
			)
			if gatewayConfig == nil {
				return fmt.Errorf("no gateway config found")
			}
			if gatewayConfig.Spec.Storage.Type != v1beta1.StorageTypeEtcd {
				return fmt.Errorf("storage type is not etcd")
			}
			if gatewayConfig.Spec.Storage.Etcd.Certs == nil {
				return fmt.Errorf("etcd config is missing certs")
			}
			endpoints := gatewayConfig.Spec.Storage.Etcd.Endpoints
			cert := gatewayConfig.Spec.Storage.Etcd.Certs.ClientCert
			key := gatewayConfig.Spec.Storage.Etcd.Certs.ClientKey
			ca := gatewayConfig.Spec.Storage.Etcd.Certs.ServerCA

			os.Args = append([]string{"etcdctl",
				fmt.Sprintf("--endpoints=%s", strings.Join(endpoints, ",")),
				fmt.Sprintf("--cacert=%s", ca),
				fmt.Sprintf("--cert=%s", cert),
				fmt.Sprintf("--key=%s", key),
			}, args...)
			return ctlv3.Start()
		},
	}
	return debugEtcdctlCmd
}
