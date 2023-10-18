package dryrun

import (
	"context"
	"fmt"
	"reflect"
	strings "strings"

	"github.com/nsf/jsondiff"
	cliutil "github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	cobra "github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildCmd[
	T driverutil.ConfigType[T],
	G driverutil.GetRequestType,
	S driverutil.SetRequestType[T],
	R driverutil.ResetRequestType[T],
	D driverutil.DryRunRequestType[T],
	DR driverutil.DryRunResponseType[T],
	H driverutil.HistoryRequestType,
	HR driverutil.HistoryResponseType[T],
	I driverutil.ClientContextInjector[C],
	C interface {
		driverutil.GetClient[T, G]
		driverutil.SetClient[T, S]
		driverutil.ResetClient[T, R]
		driverutil.DryRunClient[T, D, DR]
		driverutil.HistoryClient[T, H, HR]
	},
](use string, cci I, dryRunnableCmds ...*cobra.Command) *cobra.Command {
	var diffFull bool
	var diffFormat string
	dryRunCmd := &cobra.Command{
		Use: "config dry-run",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cliutil.BasePreRunE(cmd, args); err != nil {
				return err
			}
			// inject the dry-run client into the context
			client, ok := cci.ClientFromContext(cmd.Context())
			if ok {
				cmd.SetContext(cci.ContextWithClient(cmd.Context(), interface {
					driverutil.GetClient[T, G]
					driverutil.SetClient[T, S]
					driverutil.ResetClient[T, R]
					driverutil.DryRunClient[T, D, DR]
					driverutil.HistoryClient[T, H, HR]
				}(&DryRunClient[T, G, S, R, D, DR, H, HR, C]{
					Client: client,
				}).(C)))
			}
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if err := cliutil.BasePostRunE(cmd, args); err != nil {
				return err
			}
			// print the dry-run response
			client, ok := cci.ClientFromContext(cmd.Context())
			if !ok {
				return nil
			}
			dryRunClient, ok := interface {
				driverutil.GetClient[T, G]
				driverutil.SetClient[T, S]
				driverutil.ResetClient[T, R]
				driverutil.DryRunClient[T, D, DR]
				driverutil.HistoryClient[T, H, HR]
			}(client).(*DryRunClient[T, G, S, R, D, DR, H, HR, C])
			if !ok {
				return nil
			}
			response := dryRunClient.Response
			if errs := response.GetValidationErrors(); len(errs) > 0 {
				cmd.Println(fmt.Sprintf("validation errors occurred (%d):", len(errs)))
				for _, e := range errs {
					switch e.GetSeverity() {
					case driverutil.ValidationError_Warning:
						cmd.Print("[" + chalk.Yellow.Color("WARN") + "] ")
					case driverutil.ValidationError_Error:
						cmd.Print("[" + chalk.Red.Color("ERROR") + "] ")
					}
					cmd.Println(e.GetMessage())
				}
			}

			var opts jsondiff.Options
			switch diffFormat {
			case "console":
				opts = jsondiff.DefaultConsoleOptions()
			case "json":
				opts = jsondiff.DefaultJSONOptions()
			case "html":
				opts = jsondiff.DefaultHTMLOptions()
			default:
				return fmt.Errorf("invalid diff format: %s", diffFormat)
			}
			opts.SkipMatches = !diffFull

			str, anyChanges := driverutil.RenderJsonDiff(response.GetCurrent(), response.GetModified(), opts)
			if !anyChanges {
				cmd.Println("no changes")
			} else {
				cmd.Println(str)
			}
			return nil
		},
	}
	dryRunCmd.PersistentFlags().BoolVar(&diffFull, "diff-full", false, "show full diff, including all unchanged fields")
	dryRunCmd.PersistentFlags().StringVar(&diffFormat, "diff-format", "console", "diff format (console, json, html)")

	// if all commands have multiple words with the same first word, trim the first word
	maybeParentCommand := ""
	for _, cmd := range dryRunnableCmds {
		if words := strings.SplitAfter(cmd.Use, " "); len(words) > 1 {
			if maybeParentCommand == "" || maybeParentCommand == words[0] {
				maybeParentCommand = words[0]
			} else {
				maybeParentCommand = ""
				break
			}
		}
		cmd.Use = strings.TrimPrefix(cmd.Use, maybeParentCommand)
		cmd.Short = fmt.Sprintf("[dry-run] %s", cmd.Short)
		dryRunCmd.AddCommand(cmd)
	}
	return dryRunCmd
}

type DryRunClient[
	T driverutil.ConfigType[T],
	G driverutil.GetRequestType,
	S driverutil.SetRequestType[T],
	R driverutil.ResetRequestType[T],
	D driverutil.DryRunRequestType[T],
	DR driverutil.DryRunResponseType[T],
	H driverutil.HistoryRequestType,
	HR driverutil.HistoryResponseType[T],
	C interface {
		driverutil.GetClient[T, G]
		driverutil.SetClient[T, S]
		driverutil.ResetClient[T, R]
		driverutil.DryRunClient[T, D, DR]
		driverutil.HistoryClient[T, H, HR]
	},
] struct {
	Client   C
	Request  D
	Response DR

	installable *bool
}

func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) isInstallableConfigType() bool {
	if dc.installable != nil {
		return *dc.installable
	}
	var t T
	installable := reflect.TypeOf(t).Implements(reflect.TypeOf((*driverutil.InstallableConfigType[T])(nil)).Elem())
	dc.installable = &installable
	return installable
}

// ResetConfiguration implements driverutil.GetClient[T, G].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) ResetConfiguration(ctx context.Context, req R, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	dc.Request = NewDryRunRequest[T, D]().
		Active().
		Reset().
		Revision(req.GetRevision()).
		Patch(req.GetPatch()).
		Mask(req.GetMask()).
		Build()

	var err error
	dc.Response, err = dc.Client.DryRun(ctx, dc.Request, opts...)
	if err != nil {
		return nil, fmt.Errorf("[dry-run] error: %w", err)
	}
	return &emptypb.Empty{}, nil
}

// ResetDefaultConfiguration implements driverutil.ResetClient[T, R].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) ResetDefaultConfiguration(ctx context.Context, _ *emptypb.Empty, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	dc.Request = NewDryRunRequest[T, D]().
		Default().
		Reset().
		Build()

	var err error
	dc.Response, err = dc.Client.DryRun(ctx, dc.Request, opts...)
	if err != nil {
		return nil, fmt.Errorf("[dry-run] error: %w", err)
	}
	return &emptypb.Empty{}, nil
}

// SetConfiguration implements driverutil.SetClient[T, S].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) SetConfiguration(ctx context.Context, in S, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if dc.isInstallableConfigType() {
		in.GetSpec().ProtoReflect().Clear(util.FieldByName[T]("enabled"))
	}
	dc.Request = NewDryRunRequest[T, D]().
		Active().
		Set().
		Spec(in.GetSpec()).
		Build()

	var err error
	dc.Response, err = dc.Client.DryRun(ctx, dc.Request, opts...)
	if err != nil {
		return nil, fmt.Errorf("[dry-run] error: %w", err)
	}
	return &emptypb.Empty{}, nil
}

// SetDefaultConfiguration implements driverutil.SetClient[T, S].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) SetDefaultConfiguration(ctx context.Context, in S, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	if dc.isInstallableConfigType() {
		in.GetSpec().ProtoReflect().Clear(util.FieldByName[T]("enabled"))
	}
	dc.Request = NewDryRunRequest[T, D]().
		Default().
		Set().
		Spec(in.GetSpec()).
		Build()

	var err error
	dc.Response, err = dc.Client.DryRun(ctx, dc.Request, opts...)
	if err != nil {
		return nil, fmt.Errorf("[dry-run] error: %w", err)
	}
	return &emptypb.Empty{}, nil
}

// GetConfiguration implements driverutil.GetClient[T, G].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) GetConfiguration(ctx context.Context, in G, opts ...grpc.CallOption) (T, error) {
	return dc.Client.GetConfiguration(ctx, in, opts...)
}

// GetDefaultConfiguration implements driverutil.GetClient[T, G].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) GetDefaultConfiguration(ctx context.Context, in G, opts ...grpc.CallOption) (T, error) {
	return dc.Client.GetDefaultConfiguration(ctx, in, opts...)
}

// Install implements driverutil.InstallerClient.
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) Install(ctx context.Context, _ *emptypb.Empty, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	spec := util.NewMessage[T]()
	if dc.isInstallableConfigType() {
		spec.ProtoReflect().Set(util.FieldByName[T]("enabled"), protoreflect.ValueOfBool(true))
	}
	dc.Request = NewDryRunRequest[T, D]().
		Active().
		Set().
		Spec(spec).
		Build()

	var err error
	dc.Response, err = dc.Client.DryRun(ctx, dc.Request, opts...)
	if err != nil {
		return nil, fmt.Errorf("[dry-run] error: %w", err)
	}
	return &emptypb.Empty{}, nil
}

// Uninstall implements driverutil.InstallerClient.
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) Uninstall(ctx context.Context, _ *emptypb.Empty, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	spec := util.NewMessage[T]()
	if dc.isInstallableConfigType() {
		spec.ProtoReflect().Set(util.FieldByName[T]("enabled"), protoreflect.ValueOfBool(false))
	}
	dc.Request = NewDryRunRequest[T, D]().
		Active().
		Set().
		Spec(spec).
		Build()

	var err error
	dc.Response, err = dc.Client.DryRun(ctx, dc.Request, opts...)
	if err != nil {
		return nil, fmt.Errorf("[dry-run] error: %w", err)
	}
	return &emptypb.Empty{}, nil
}

// DryRun implements driverutil.DryRunClient[T, D, DR].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) DryRun(_ context.Context, _ D, _ ...grpc.CallOption) (DR, error) {
	return lo.Empty[DR](), status.Errorf(codes.Unimplemented, "[dry-run] method DryRun not implemented")
}

// ConfigurationHistory implements driverutil.HistoryClient[T, H, HR].
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) ConfigurationHistory(_ context.Context, _ H, _ ...grpc.CallOption) (HR, error) {
	return lo.Empty[HR](), status.Errorf(codes.Unimplemented, "[dry-run] method ConfigurationHistory not implemented")
}

// Status implements driverutil.InstallerClient.
func (dc *DryRunClient[T, G, S, R, D, DR, H, HR, C]) Status(_ context.Context, _ *emptypb.Empty, _ ...grpc.CallOption) (*driverutil.InstallStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "[dry-run] method Status not implemented")
}
