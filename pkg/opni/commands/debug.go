//go:build !minimal

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	channelzcmd "github.com/kazegusuri/channelzcli/cmd"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/etcdctl/v3/ctlv3"
	channelzgrpc "google.golang.org/grpc/channelz/grpc_channelz_v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	debugCmd.AddCommand(BuildDebugChannelzCmd())
	debugCmd.AddCommand(BuildDebugDashboardSettingsCmd())
	debugCmd.AddCommand(BuildDebugImportAgentCmd())
	debugCmd.AddCommand(BuildDebugAgentLogStreamGetCmd())
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
					lg.Warn(fmt.Sprintf("failed to create management client: %v", err))
				}
			}
			conf, err := mgmtClient.GetConfig(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			objects, err := machinery.LoadDocuments(conf.Documents)
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

func BuildDebugChannelzCmd() *cobra.Command {
	globalOpts := &channelzcmd.GlobalOptions{
		Insecure: true,
		Input:    os.Stdin,
		Output:   os.Stdout,
	}
	cmd := &cobra.Command{
		Use:   "channelz",
		Short: "Interact with the gateway's channelz service",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			globalOpts.Address = cmd.Flag("address").Value.String()
		},
	}
	descCmd := channelzcmd.NewDescribeCommand(globalOpts).Command()
	descCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"channel", "server", "serversocket"}, cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			if err := managementPreRunE(cmd, nil); err != nil {
				return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
			}
			client := channelzgrpc.NewChannelzClient(managementv1.UnderlyingConn(mgmtClient))
			nameIdPairs := []lo.Tuple2[string, string]{}
			nameLookupEnabled := false
			switch args[0] {
			case "channel":
				nameLookupEnabled = true
				channels, err := client.GetTopChannels(context.Background(), &channelzgrpc.GetTopChannelsRequest{})
				if err != nil {
					return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
				}
				for _, ch := range channels.GetChannel() {
					name := ch.Ref.GetName()
					id := fmt.Sprint(ch.Ref.GetChannelId())
					nameIdPairs = append(nameIdPairs, lo.T2(name, id))
				}
			case "server":
				servers, err := client.GetServers(cmd.Context(), &channelzgrpc.GetServersRequest{})
				if err != nil {
					return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
				}
				for _, srv := range servers.GetServer() {
					name := srv.Ref.GetName()
					if name == "" && len(srv.ListenSocket) > 0 {
						if res, err := client.GetSocket(cmd.Context(), &channelzgrpc.GetSocketRequest{SocketId: srv.ListenSocket[0].SocketId}); err == nil {
							if local := res.GetSocket().GetLocal().GetTcpipAddress(); local != nil {
								name = fmt.Sprintf("[%v]:%v", net.IP(local.IpAddress).String(), local.Port)
							}
						}
					}
					id := fmt.Sprint(srv.Ref.GetServerId())
					nameIdPairs = append(nameIdPairs, lo.T2(name, id))
				}
			case "serversocket":
				servers, err := client.GetServers(cmd.Context(), &channelzgrpc.GetServersRequest{})
				if err != nil {
					return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
				}
				for _, srv := range servers.GetServer() {
					serverSockets, err := client.GetServerSockets(cmd.Context(), &channelzgrpc.GetServerSocketsRequest{
						ServerId: srv.Ref.GetServerId(),
					})
					if err != nil {
						continue
					}
					for _, sock := range serverSockets.GetSocketRef() {
						name := sock.GetName()
						id := fmt.Sprint(sock.GetSocketId())
						nameIdPairs = append(nameIdPairs, lo.T2(name, id))
					}
				}
			default:
				return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
			}
			comps := []string{}
			for _, pair := range nameIdPairs {
				name, id := pair.Unpack()
				if name == "unused" {
					name = ""
				}
				if nameLookupEnabled && name != "" && strings.HasPrefix(name, toComplete) {
					comps = append(comps, fmt.Sprintf("%s\t%s", name, id))
				} else if strings.HasPrefix(id, toComplete) {
					if len(name) > 0 {
						comps = append(comps, fmt.Sprintf("%s\t%s", id, name))
					} else {
						comps = append(comps, id)
					}
				}
			}
			return comps, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmd.AddCommand(descCmd)
	listCmd := channelzcmd.NewListCommand(globalOpts).Command()
	listCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"channel", "server", "serversocket"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmd.AddCommand(listCmd)
	treeCmd := channelzcmd.NewTreeCommand(globalOpts).Command()
	treeCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"channel", "server"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmd.AddCommand(treeCmd)
	return cmd
}

func BuildDebugDashboardSettingsCmd() *cobra.Command {
	debugDashboardSettingsCmd := &cobra.Command{
		Use:   "dashboard-settings",
		Short: "Manage dashboard settings",
	}
	debugDashboardSettingsCmd.AddCommand(BuildDebugDashboardSettingsGetCmd())
	debugDashboardSettingsCmd.AddCommand(BuildDebugDashboardSettingsUpdateCmd())
	return debugDashboardSettingsCmd
}

func BuildDebugDashboardSettingsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the dashboard settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := mgmtClient.GetDashboardSettings(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			data, err := protojson.MarshalOptions{
				Multiline:       true,
				EmitUnpopulated: true,
			}.Marshal(settings)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		},
	}
	return cmd
}

func BuildDebugDashboardSettingsUpdateCmd() *cobra.Command {
	var reset bool
	var defaultImageRepository string
	var defaultTokenTtl string
	var defaultTokenLabels []string
	var userSettings []string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update dashboard settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var settings *managementv1.DashboardSettings
			if reset {
				settings = &managementv1.DashboardSettings{}
			} else {
				var err error
				settings, err = mgmtClient.GetDashboardSettings(cmd.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
			}

			if settings.Global == nil {
				settings.Global = &managementv1.DashboardGlobalSettings{}
			}
			if defaultImageRepository != "" {
				settings.Global.DefaultImageRepository = defaultImageRepository
			}
			if defaultTokenTtl != "" {
				d, err := time.ParseDuration(defaultTokenTtl)
				if err != nil {
					return err
				}
				settings.Global.DefaultTokenTtl = durationpb.New(d)
			}
			if defaultTokenLabels != nil {
				kv, err := cliutil.ParseKeyValuePairs(defaultTokenLabels)
				if err != nil {
					return err
				}
				settings.Global.DefaultTokenLabels = kv
			}

			if userSettings != nil {
				kv, err := cliutil.ParseKeyValuePairs(userSettings)
				if err != nil {
					return err
				}
				settings.User = kv
			}

			_, err := mgmtClient.UpdateDashboardSettings(cmd.Context(), settings)
			if err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&defaultImageRepository, "global.default-image-repository", "", "Default image repository for helm command templates")
	cmd.Flags().StringVar(&defaultTokenTtl, "global.default-token-ttl", "", "Default token TTL")
	cmd.Flags().StringSliceVar(&defaultTokenLabels, "global.default-token-labels", nil, "Default token labels (key-value pairs)")
	cmd.Flags().StringSliceVar(&userSettings, "user", []string{}, "User settings (key-value pairs)")
	cmd.Flags().BoolVar(&reset, "reset", false, "Reset settings to default values. If other flags are specified, they will be applied on top of the default values.")

	return cmd
}

func BuildDebugImportAgentCmd() *cobra.Command {
	var agentId, keyringFile string
	cmd := &cobra.Command{
		Use:   "import-agent",
		Short: "Import a missing or deleted agent and keyring",
		RunE: func(cmd *cobra.Command, args []string) error {
			var keyringData []byte
			var err error
			if keyringFile == "-" {
				keyringData, err = io.ReadAll(os.Stdin)
			} else {
				keyringData, err = os.ReadFile(keyringFile)
			}
			if err != nil {
				return fmt.Errorf("failed to read keyring: %w", err)
			}
			kr, err := keyring.Unmarshal(keyringData)
			if err != nil {
				return fmt.Errorf("failed to unmarshal keyring: %w", err)
			}

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

			_, err = backend.GetCluster(context.Background(), &corev1.Reference{Id: agentId})
			if err == nil {
				return fmt.Errorf("agent %q already exists", agentId)
			} else if !storage.IsNotFound(err) {
				return fmt.Errorf("storage backend error: %w", err)
			}

			newCluster := &corev1.Cluster{
				Id: agentId,
			}
			if err := backend.CreateCluster(context.Background(), newCluster); err != nil {
				return fmt.Errorf("failed to create cluster: %w", err)
			}

			krStore := backend.KeyringStore("gateway", newCluster.Reference())
			if err := krStore.Put(context.Background(), kr); err != nil {
				return fmt.Errorf("failed to store keyring: %w", err)
			}

			cmd.Println("Agent and keyring imported successfully")

			return nil
		},
	}
	cmd.Flags().StringVar(&agentId, "id", "", "Agent ID")
	cmd.Flags().StringVar(&keyringFile, "keyring", "", "Path to a keyring file")
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("keyring")

	return cmd
}

func BuildDebugAgentLogStreamGetCmd() *cobra.Command {
	var since, until, level, output string
	var follow bool
	var names []string
	cmd := &cobra.Command{
		Use:   "agent-logs <cluster-id>",
		Short: "Get agent logs",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					lg.Error("fatal", err)
					os.Exit(1)
				}
				for _, c := range cl.Items {
					args = append(args, c.Id)
				}
			}
			streamRequest := &managementv1.StreamAgentLogsRequest{
				Agent: &corev1.Reference{
					Id: args[0],
				},
				Request: &controlv1.LogStreamRequest{
					Filters: &controlv1.LogStreamFilters{
						NamePattern: names,
						Level:       lo.ToPtr(int32(logger.ParseLevel(level))),
					},
					Follow: follow,
				},
			}

			startTime := parseTimeOrDie(since)
			endTime := parseTimeOrDie(until)
			streamRequest.Request.Since = timestamppb.New(startTime)
			streamRequest.Request.Until = timestamppb.New(endTime)

			stream, err := mgmtClient.GetAgentLogStream(cmd.Context(), streamRequest)
			if err != nil {
				return err
			}
			for {
				log, err := stream.Recv()
				if err != nil {
					return err
				}

				done := (proto.Equal(log, &controlv1.StructuredLogRecord{}))
				keepFollowing := done && follow
				if keepFollowing {
					continue
				} else if done {
					return nil
				}

				var attrs strings.Builder
				for _, attr := range log.Attributes {
					attrs.WriteString(attr.Key)
					attrs.WriteString("=")
					attrs.WriteString(attr.Value)
					attrs.WriteString(" ")
				}
				fmt.Println(log.Time.AsTime().Format(logger.DefaultTimeFormat), log.Level, log.Name, log.Source, log.Message, attrs.String())
			}
		},
	}

	cmd.Flags().StringVar(&since, "since", "1 day ago", "Start time")
	cmd.Flags().StringVar(&until, "until", "now", "End time")
	cmd.Flags().StringSliceVar(&names, "name", nil, "Pattern filter(s) by component name")
	cmd.Flags().StringVar(&level, "level", "info", "Minimum log level severity (debug, info, warn, error)")
	cmd.Flags().StringVar(&output, "output", "text", "Output format")
	cmd.Flags().BoolVar(&follow, "follow", false, "Follow logs")
	cmd.RegisterFlagCompletionFunc("level", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveDefault
	})
	cmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"text", "json"}, cobra.ShellCompDirectiveDefault
	})
	return cmd
}

func init() {
	AddCommandsToGroup(Debug, BuildDebugCmd())
}
