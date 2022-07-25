package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
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
			config, err := mgmtClient.GetConfig(cmd.Context(), &emptypb.Empty{})
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
			config, err := mgmtClient.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			docsNoSchema := []*managementv1.ConfigDocument{}
			for _, doc := range config.Documents {
				docsNoSchema = append(docsNoSchema, &managementv1.ConfigDocument{
					Json: doc.Json,
				})
			}
			_, err = mgmtClient.UpdateConfig(cmd.Context(), &managementv1.UpdateConfigRequest{
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
		Long:               "To specify a gateway address, use the OPNI_ADDRESS environment variable.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := os.Getenv("OPNI_ADDRESS")
			var gatewayConfig *v1beta1.GatewayConfig
			if addr != "" {
				c, err := clients.NewManagementClient(cmd.Context(), clients.WithAddress(addr))
				if err == nil {
					mgmtClient = c
				} else {
					lg.Warnf("failed to create management client: %v", err)
				}
			}
			conf, err := mgmtClient.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			objects, err := config.LoadObjects(conf.YAMLDocuments())
			if err != nil {
				return err
			}
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
			endpoints := gatewayConfig.Spec.Storage.Etcd.Endpoints
			argv := []string{"etcdctl", fmt.Sprintf("--endpoints=%s", strings.Join(endpoints, ","))}
			if gatewayConfig.Spec.Storage.Etcd.Certs != nil {
				cert := gatewayConfig.Spec.Storage.Etcd.Certs.ClientCert
				key := gatewayConfig.Spec.Storage.Etcd.Certs.ClientKey
				ca := gatewayConfig.Spec.Storage.Etcd.Certs.ServerCA
				argv = append(argv,
					fmt.Sprintf("--cacert=%s", ca),
					fmt.Sprintf("--cert=%s", cert),
					fmt.Sprintf("--key=%s", key),
				)
			}

			os.Args = append(argv, args...)
			return ctlv3.Start()
		},
	}
	return debugEtcdctlCmd
}
