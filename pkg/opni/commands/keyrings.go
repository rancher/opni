package commands

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/rancher/opni/apis/v1beta2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/util"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildKeyringsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "keyrings",
		Short:   "Manage keyrings",
		Aliases: []string{"keyring"},
	}
	cmd.AddCommand(BuildKeyringsGetCmd())
	ConfigureManagementCommand(cmd)
	return cmd
}

func BuildKeyringsGetCmd() *cobra.Command {
	var output, encoding string
	var padding, indent bool
	cmd := &cobra.Command{
		Use:   "get <cluster-id>",
		Short: "get keyring data for a cluster",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete)
		},
		PreRun: func(*cobra.Command, []string) {
			logger.DefaultLogLevel.SetLevel(zapcore.WarnLevel)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := mgmtClient.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return fmt.Errorf("failed to get config: %w", err)
			}
			objects, err := config.LoadObjects(resp.YAMLDocuments())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			var gatewayConf *v1beta1.GatewayConfig
			ok := objects.Visit(func(gc *v1beta1.GatewayConfig) {
				if gatewayConf == nil {
					gatewayConf = gc
				}
			})
			if !ok {
				return fmt.Errorf("gateway config not found")
			}

			backend, err := machinery.ConfigureStorageBackend(cmd.Context(), &gatewayConf.Spec.Storage)
			if err != nil {
				return fmt.Errorf("failed to configure storage backend: %w", err)
			}

			store := backend.KeyringStore("gateway", &corev1.Reference{
				Id: args[0],
			})

			kr, err := store.Get(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get keyring: %w", err)
			}

			switch output {
			case "json":
				keys := map[string][]any{}
				kr.ForEach(func(k any) {
					keys[reflect.TypeOf(k).Name()] = append(keys[reflect.TypeOf(k).Name()], k)
				})
				if indent {
					jsonData := util.Must(kr.Marshal())
					indented := new(bytes.Buffer)
					json.Indent(indented, jsonData, "", "  ")
					fmt.Println(indented.String())
				} else {
					fmt.Println(string(util.Must(kr.Marshal())))
				}
			case "base64":
				var e *base64.Encoding
				switch encoding {
				case "url":
					e = base64.URLEncoding
				case "std":
					e = base64.StdEncoding
				default:
					return fmt.Errorf("invalid encoding: %s", encoding)
				}
				if !padding {
					e = e.WithPadding(base64.NoPadding)
				}
				fmt.Println(e.EncodeToString(util.Must(kr.Marshal())))
			case "crd":
				fmt.Printf("apiVersion: %s\nkind: %s\nmetadata:\n  name: %s\ndata: %s\n",
					v1beta2.GroupVersion.String(), "Keyring", args[0],
					base64.StdEncoding.EncodeToString(util.Must(kr.Marshal())))
			default:
				return fmt.Errorf("unsupported encoding: %s", output)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format. One of: json|base64|crd")
	cmd.Flags().BoolVar(&indent, "indent", true, "Indent JSON output")
	cmd.Flags().StringVar(&encoding, "encoding", "url", "Base64 encoding. One of: std|url")
	cmd.Flags().BoolVar(&padding, "padding", false, "Print base64 data with padding characters")
	return cmd
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildKeyringsCmd())
}
