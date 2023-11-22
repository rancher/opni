// Code generated by internal/codegen/cli/generator.go. DO NOT EDIT.
// source: github.com/rancher/opni/plugins/metrics/apis/node/config.proto

package node

import (
	context "context"
	errors "errors"
	cli "github.com/rancher/opni/internal/codegen/cli"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	v1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	cliutil "github.com/rancher/opni/pkg/opni/cliutil"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	flagutil "github.com/rancher/opni/pkg/util/flagutil"
	cobra "github.com/spf13/cobra"
	pflag "github.com/spf13/pflag"
	grpc "google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	strings "strings"
)

type (
	contextKey_NodeConfiguration_type      struct{}
	contextInjector_NodeConfiguration_type struct{}
)

var (
	contextKey_NodeConfiguration     contextKey_NodeConfiguration_type
	NodeConfigurationContextInjector contextInjector_NodeConfiguration_type
)

func (contextInjector_NodeConfiguration_type) NewClient(cc grpc.ClientConnInterface) NodeConfigurationClient {
	return NewNodeConfigurationClient(cc)
}

func (contextInjector_NodeConfiguration_type) UnderlyingConn(client NodeConfigurationClient) grpc.ClientConnInterface {
	return client.(*nodeConfigurationClient).cc
}

func (contextInjector_NodeConfiguration_type) ContextWithClient(ctx context.Context, client NodeConfigurationClient) context.Context {
	return context.WithValue(ctx, contextKey_NodeConfiguration, client)
}

func (contextInjector_NodeConfiguration_type) ClientFromContext(ctx context.Context) (NodeConfigurationClient, bool) {
	client, ok := ctx.Value(contextKey_NodeConfiguration).(NodeConfigurationClient)
	return client, ok
}

var extraCmds_NodeConfiguration []*cobra.Command

func addExtraNodeConfigurationCmd(custom *cobra.Command) {
	extraCmds_NodeConfiguration = append(extraCmds_NodeConfiguration, custom)
}

func BuildNodeConfigurationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: `The NodeConfiguration service allows for per-node configuration of the`,
		Long: `
metrics capability.
Served as a management API extension.
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
	}

	cliutil.AddSubcommands(cmd, append([]*cobra.Command{
		BuildNodeConfigurationGetDefaultConfigurationCmd(),
		BuildNodeConfigurationSetDefaultConfigurationCmd(),
		BuildNodeConfigurationResetDefaultConfigurationCmd(),
		BuildNodeConfigurationGetConfigurationCmd(),
		BuildNodeConfigurationSetConfigurationCmd(),
		BuildNodeConfigurationResetConfigurationCmd(),
		BuildNodeConfigurationConfigurationHistoryCmd(),
	}, extraCmds_NodeConfiguration...)...)
	cli.AddOutputFlag(cmd)
	return cmd
}

var buildHooks_NodeConfigurationGetDefaultConfiguration []func(*cobra.Command)

func addBuildHook_NodeConfigurationGetDefaultConfiguration(hook func(*cobra.Command)) {
	buildHooks_NodeConfigurationGetDefaultConfiguration = append(buildHooks_NodeConfigurationGetDefaultConfiguration, hook)
}

func BuildNodeConfigurationGetDefaultConfigurationCmd() *cobra.Command {
	in := &GetRequest{}
	cmd := &cobra.Command{
		Use:   "get-default",
		Short: "Returns the default implementation-specific configuration, or one previously set.",
		Long: `
If a default configuration was previously set using SetDefaultConfiguration, it
returns that configuration. Otherwise, returns implementation-specific defaults.

An optional revision argument can be provided to get a specific historical
version of the configuration instead of the current configuration.

HTTP handlers for this method:
- GET /node_config
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			if in == nil {
				return errors.New("no input provided")
			}
			response, err := client.GetDefaultConfiguration(cmd.Context(), in)
			if err != nil {
				return err
			}
			cli.RenderOutput(cmd, response)
			return nil
		},
	}
	cmd.Flags().AddFlagSet(in.FlagSet())
	for _, hook := range buildHooks_NodeConfigurationGetDefaultConfiguration {
		hook(cmd)
	}
	return cmd
}

func BuildNodeConfigurationSetDefaultConfigurationCmd() *cobra.Command {
	in := &SetRequest{}
	cmd := &cobra.Command{
		Use:   "set-default",
		Short: "Sets the default configuration that will be used as the base for future configuration changes.",
		Long: `
If no custom default configuration is set using this method, implementation-specific
defaults may be chosen.

Unlike with SetConfiguration, the input is not merged with the existing configuration,
instead replacing it directly.

If the revision field is set, the server will reject the request if the current
revision does not match the provided revision.

This API is different from the SetConfiguration API, and should not be necessary
for most use cases. It can be used in situations where an additional persistence
layer that is not driver-specific is desired.

HTTP handlers for this method:
- PUT /node_config
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			if cmd.Flags().Lookup("interactive").Value.String() == "true" {
				if curValue, err := client.GetDefaultConfiguration(cmd.Context(), &GetRequest{}); err == nil {
					in.Spec = curValue
				}
				if edited, err := cliutil.EditInteractive(in.Spec); err != nil {
					return err
				} else {
					in.Spec = edited
				}
			} else if fileName := cmd.Flags().Lookup("file").Value.String(); fileName != "" {
				if in.Spec == nil {
					cliutil.InitializeField(&in.Spec)
				}
				if err := cliutil.LoadFromFile(in.Spec, fileName); err != nil {
					return err
				}
			}
			if in == nil {
				return errors.New("no input provided")
			}
			_, err := client.SetDefaultConfiguration(cmd.Context(), in)
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringP("file", "f", "", "path to a file containing the config, or - to read from stdin")
	cmd.Flags().BoolP("interactive", "i", false, "edit the config interactively in an editor")
	cmd.MarkFlagsMutuallyExclusive("file", "interactive")
	cmd.MarkFlagFilename("file")
	return cmd
}

func BuildNodeConfigurationResetDefaultConfigurationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset-default",
		Short: "Resets the default configuration to the implementation-specific defaults.",
		Long: `
If a custom default configuration was previously set using SetDefaultConfiguration,
it will be replaced with the implementation-specific defaults. Otherwise,
this will have no effect.

HTTP handlers for this method:
- POST /node_config/reset
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			_, err := client.ResetDefaultConfiguration(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

var buildHooks_NodeConfigurationGetConfiguration []func(*cobra.Command)

func addBuildHook_NodeConfigurationGetConfiguration(hook func(*cobra.Command)) {
	buildHooks_NodeConfigurationGetConfiguration = append(buildHooks_NodeConfigurationGetConfiguration, hook)
}

func BuildNodeConfigurationGetConfigurationCmd() *cobra.Command {
	in := &GetRequest{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Gets the current configuration, or the default configuration if not set.",
		Long: `
This configuration is maintained and versioned separately from the default
configuration, and has different semantics regarding merging and persistence.

The active configuration can be set using SetConfiguration. Then, future
calls to GetConfiguration will return that configuration instead of falling
back to the default.

An optional revision argument can be provided to get a specific historical
version of the configuration instead of the current configuration.
This revision value can be obtained from the revision field of a previous
call to GetConfiguration, or from the revision field of one of the history
entries returned by GetConfigurationHistory.

HTTP handlers for this method:
- GET /node_config/{node.id}
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			if in == nil {
				return errors.New("no input provided")
			}
			response, err := client.GetConfiguration(cmd.Context(), in)
			if err != nil {
				return err
			}
			cli.RenderOutput(cmd, response)
			return nil
		},
	}
	cmd.Flags().AddFlagSet(in.FlagSet())
	for _, hook := range buildHooks_NodeConfigurationGetConfiguration {
		hook(cmd)
	}
	return cmd
}

var buildHooks_NodeConfigurationSetConfiguration []func(*cobra.Command)

func addBuildHook_NodeConfigurationSetConfiguration(hook func(*cobra.Command)) {
	buildHooks_NodeConfigurationSetConfiguration = append(buildHooks_NodeConfigurationSetConfiguration, hook)
}

func BuildNodeConfigurationSetConfigurationCmd() *cobra.Command {
	in := &SetRequest{}
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Updates the active configuration by merging the input with the current active configuration.",
		Long: `
If there is no active configuration, the input will be merged with the default configuration.

The merge is performed by replacing all *present* fields in the input with the
corresponding fields in the target. Slices and maps are overwritten and not combined.
Any *non-present* fields in the input are ignored, and the corresponding fields
in the target are left unchanged.

Field presence is defined by the protobuf spec. The following kinds of fields
have presence semantics:
- Messages
- Repeated fields (scalars or messages)
- Maps
- Optional scalars
Non-optional scalars do *not* have presence semantics, and are always treated
as present for the purposes of merging. For this reason, it is not recommended
to use non-optional scalars in messages intended to be used with this API.

Subsequent calls to this API will merge inputs with the previous active configuration,
not the default configuration.

When updating an existing configuration, the revision number in the input configuration
must match the revision number of the existing configuration, otherwise a conflict
error will be returned. The timestamp field of the revision is ignored for this purpose.

Some fields in the configuration may be marked as secrets. These fields are
write-only from this API, and the placeholder value "***" will be returned in
place of the actual value when getting the configuration.
When setting the configuration, the same placeholder value can be used to indicate
the existing value should be preserved.

HTTP handlers for this method:
- PUT /node_config/{node.id}
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			if cmd.Flags().Lookup("interactive").Value.String() == "true" {
				if curValue, err := client.GetConfiguration(cmd.Context(), &GetRequest{}); err == nil {
					in.Spec = curValue
				}
				if edited, err := cliutil.EditInteractive(in.Spec); err != nil {
					return err
				} else {
					in.Spec = edited
				}
			} else if fileName := cmd.Flags().Lookup("file").Value.String(); fileName != "" {
				if in.Spec == nil {
					cliutil.InitializeField(&in.Spec)
				}
				if err := cliutil.LoadFromFile(in.Spec, fileName); err != nil {
					return err
				}
			}
			if in == nil {
				return errors.New("no input provided")
			}
			_, err := client.SetConfiguration(cmd.Context(), in)
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringP("file", "f", "", "path to a file containing the config, or - to read from stdin")
	cmd.Flags().BoolP("interactive", "i", false, "edit the config interactively in an editor")
	cmd.MarkFlagsMutuallyExclusive("file", "interactive")
	cmd.MarkFlagFilename("file")
	for _, hook := range buildHooks_NodeConfigurationSetConfiguration {
		hook(cmd)
	}
	return cmd
}

func BuildNodeConfigurationResetConfigurationCmd() *cobra.Command {
	in := &ResetRequest{}
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Resets the active configuration to the current default configuration.",
		Long: `
The request may optionally contain a field mask to specify which fields should
be preserved. Furthermore, if a mask is set, the request may also contain a patch
object used to apply additional changes to the masked fields. These changes are
applied atomically at the time of reset. Fields present in the patch object, but
not in the mask, are ignored.

For example, with the following message:
message Example {
 optional int32 a = 1;
 optional int32 b = 2;
 optional int32 c = 3;
}

and current state:
 active:  { a: 1, b: 2, c: 3 }
 default: { a: 4, b: 5, c: 6 }

and reset request parameters:
 {
   mask:  { paths: [ "a", "b" ] }
   patch: { a: 100 }
 }

The resulting active configuration will be:
 active: {
   a: 100, // masked, set to 100 via patch
   b: 2,   // masked, but not set in patch, so left unchanged
   c: 6,   // not masked, reset to default
 }

HTTP handlers for this method:
- POST /node_config/{node.id}/reset
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			if in == nil {
				return errors.New("no input provided")
			}
			_, err := client.ResetConfiguration(cmd.Context(), in)
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().AddFlagSet(in.FlagSet())
	return cmd
}

func BuildNodeConfigurationConfigurationHistoryCmd() *cobra.Command {
	in := &ConfigurationHistoryRequest{}
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Get a list of all past revisions of the configuration.",
		Long: `
Will return the history for either the active or default configuration
depending on the specified target.

The entries are ordered from oldest to newest, where the last entry is
the current configuration.

HTTP handlers for this method:
- GET /node_config/{node.id}/history
`[1:],
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := NodeConfigurationContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}
			if in == nil {
				return errors.New("no input provided")
			}
			response, err := client.ConfigurationHistory(cmd.Context(), in)
			if err != nil {
				return err
			}
			cli.RenderOutput(cmd, response)
			return nil
		},
	}
	cmd.Flags().AddFlagSet(in.FlagSet())
	cmd.RegisterFlagCompletionFunc("target", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"ActiveConfiguration", "DefaultConfiguration"}, cobra.ShellCompDirectiveDefault
	})
	return cmd
}

func (in *GetRequest) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("GetRequest", pflag.ExitOnError)
	fs.SortFlags = true
	if in.Node == nil {
		in.Node = &v1.Reference{}
	}
	fs.AddFlagSet(in.Node.FlagSet(append(prefix, "node")...))
	if in.Revision == nil {
		in.Revision = &v1.Revision{}
	}
	fs.AddFlagSet(in.Revision.FlagSet(append(prefix, "revision")...))
	return fs
}

func (in *SetRequest) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("SetRequest", pflag.ExitOnError)
	fs.SortFlags = true
	if in.Node == nil {
		in.Node = &v1.Reference{}
	}
	fs.AddFlagSet(in.Node.FlagSet(append(prefix, "node")...))
	if in.Spec == nil {
		in.Spec = &MetricsCapabilityConfig{}
	}
	fs.AddFlagSet(in.Spec.FlagSet(append(prefix, "spec")...))
	return fs
}

func (in *MetricsCapabilityConfig) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("MetricsCapabilityConfig", pflag.ExitOnError)
	fs.SortFlags = true
	if in.Revision == nil {
		in.Revision = &v1.Revision{}
	}
	fs.AddFlagSet(in.Revision.FlagSet(append(prefix, "revision")...))
	if in.Rules == nil {
		in.Rules = &v1beta1.RulesSpec{}
	}
	fs.AddFlagSet(in.Rules.FlagSet(append(prefix, "rules")...))
	flagutil.SetDefValue(fs, strings.Join(append(prefix, "rules", "discovery.prometheus-rules.search-namespaces"), "."), `[""]`)
	fs.Var(flagutil.EnumPtrValue(flagutil.Ptr(MetricsCapabilityConfig_Prometheus), &in.Driver), strings.Join(append(prefix, "driver"), "."), "")
	if in.Prometheus == nil {
		in.Prometheus = &PrometheusSpec{}
	}
	fs.AddFlagSet(in.Prometheus.FlagSet(append(prefix, "prometheus")...))
	if in.Otel == nil {
		in.Otel = &OTELSpec{}
	}
	fs.AddFlagSet(in.Otel.FlagSet(append(prefix, "otel")...))
	return fs
}

func (in *PrometheusSpec) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("PrometheusSpec", pflag.ExitOnError)
	fs.SortFlags = true
	fs.Var(flagutil.StringPtrValue(flagutil.Ptr("quay.io/prometheus/prometheus:latest"), &in.Image), strings.Join(append(prefix, "image"), "."), "")
	return fs
}

func (in *OTELSpec) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("OTELSpec", pflag.ExitOnError)
	fs.SortFlags = true
	if in.Wal == nil {
		in.Wal = &WALConfig{}
	}
	fs.AddFlagSet(in.Wal.FlagSet(append(prefix, "wal")...))
	fs.Var(flagutil.BoolPtrValue(nil, &in.HostMetrics), strings.Join(append(prefix, "host-metrics"), "."), "")
	return fs
}

func (in *WALConfig) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("WALConfig", pflag.ExitOnError)
	fs.SortFlags = true
	fs.Var(flagutil.BoolPtrValue(nil, &in.Enabled), strings.Join(append(prefix, "enabled"), "."), "")
	fs.Var(flagutil.IntPtrValue(nil, &in.BufferSize), strings.Join(append(prefix, "buffer-size"), "."), "")
	fs.Var(flagutil.DurationpbValue(nil, &in.TruncateFrequency), strings.Join(append(prefix, "truncate-frequency"), "."), "")
	return fs
}

func (in *ResetRequest) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("ResetRequest", pflag.ExitOnError)
	fs.SortFlags = true
	if in.Node == nil {
		in.Node = &v1.Reference{}
	}
	fs.AddFlagSet(in.Node.FlagSet(append(prefix, "node")...))
	if in.Revision == nil {
		in.Revision = &v1.Revision{}
	}
	fs.AddFlagSet(in.Revision.FlagSet(prefix...))
	return fs
}

func (in *ConfigurationHistoryRequest) FlagSet(prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("ConfigurationHistoryRequest", pflag.ExitOnError)
	fs.SortFlags = true
	if in.Node == nil {
		in.Node = &v1.Reference{}
	}
	fs.AddFlagSet(in.Node.FlagSet(append(prefix, "node")...))
	fs.Var(flagutil.EnumValue(driverutil.Target_ActiveConfiguration, &in.Target), strings.Join(append(prefix, "target"), "."), "")
	if in.Revision == nil {
		in.Revision = &v1.Revision{}
	}
	fs.AddFlagSet(in.Revision.FlagSet(prefix...))
	fs.BoolVar(&in.IncludeValues, strings.Join(append(prefix, "include-values"), "."), true, "")
	return fs
}
