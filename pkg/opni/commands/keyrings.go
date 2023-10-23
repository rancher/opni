//go:build !minimal

package commands

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"

	"slices"

	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildKeyringsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "keyrings",
		Short:   "Manage keyrings",
		Aliases: []string{"keyring"},
	}
	cmd.AddCommand(BuildKeyringsGetCmd())
	cmd.AddCommand(BuildKeyringsGenKeyCmd())
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
			logger.DefaultLogLevel = slog.LevelWarn
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := mgmtClient.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return fmt.Errorf("failed to get config: %w", err)
			}
			objects, err := machinery.LoadDocuments(resp.Documents)
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
					opnicorev1beta1.GroupVersion.String(), "Keyring", args[0],
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

func BuildKeyringsGenKeyCmd() *cobra.Command {
	supportedKeyTypes := []string{"ephemeral"}
	var usage ephemeral.KeyUsageType
	var output string
	var labels []string
	cmd := &cobra.Command{
		Use:   "gen-key <type> [flags]",
		Short: "generate a new ephemeral key",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !slices.Contains([]ephemeral.KeyUsageType{
				ephemeral.Authentication,
			}, usage) {
				return fmt.Errorf("invalid key usage: %s", usage)
			}
			keyType := args[0]
			if !slices.Contains(supportedKeyTypes, keyType) {
				return fmt.Errorf("unsupported key type: %s", keyType)
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return supportedKeyTypes, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "ephemeral":
				labels, err := cliutil.ParseKeyValuePairs(labels)
				if err != nil {
					return err
				}
				ek := ephemeral.NewKey(usage, labels)
				keyData, err := json.Marshal(ek)
				if err != nil {
					panic(err)
				}
				var file *os.File
				if output == "-" {
					file = os.Stdout
				} else {
					file, err = os.OpenFile(output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
					if err != nil {
						return err
					}
					defer file.Close()
				}

				_, err = file.Write(keyData)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported key type: %s", args[0])
			}
			return nil
		},
	}

	cmd.Flags().StringVar((*string)(&usage), "usage", "", "Key usage")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output filename")
	cmd.Flags().StringSliceVar(&labels, "labels", []string{}, "Labels to apply to the key (key=value format)")

	cmd.MarkFlagRequired("output")
	cmd.MarkFlagRequired("labels")
	cmd.MarkFlagRequired("usage")

	cmd.RegisterFlagCompletionFunc("usage", cobra.FixedCompletions([]string{
		string(ephemeral.Authentication),
	}, cobra.ShellCompDirectiveNoFileComp))

	cmd.RegisterFlagCompletionFunc("labels", cobra.NoFileCompletions)
	return cmd
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildKeyringsCmd())
}
