package example

import (
	context "context"
	"os"

	cli "github.com/rancher/opni/internal/codegen/cli"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/rollback"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tui"
	"github.com/rancher/opni/pkg/util"
	flagutil "github.com/rancher/opni/pkg/util/flagutil"
	"golang.org/x/term"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ExampleDriver interface {
	driverutil.ConfigServer[
		*ConfigSpec,
		*driverutil.GetRequest,
		*SetRequest,
		*ResetRequest,
		*driverutil.ConfigurationHistoryRequest,
		*HistoryResponse,
	]
	driverutil.DryRunServer[*ConfigSpec, *DryRunRequest, *DryRunResponse]
}

type DriverImpl struct {
	*driverutil.BaseConfigServer[
		*driverutil.GetRequest,
		*SetRequest,
		*ResetRequest,
		*driverutil.ConfigurationHistoryRequest,
		*HistoryResponse,
		*ConfigSpec,
	]
}

var drivers = driverutil.NewDriverCache[ExampleDriver]()

type ExampleDriverImplOptions struct {
	DefaultConfigStore storage.ValueStoreT[*ConfigSpec] `option:"defaultConfigStore"`
	ActiveConfigStore  storage.ValueStoreT[*ConfigSpec] `option:"activeConfigStore"`
}

func NewExampleDriverImpl(options ExampleDriverImplOptions) *DriverImpl {
	return &DriverImpl{
		BaseConfigServer: driverutil.NewBaseConfigServer[
			*driverutil.GetRequest,
			*SetRequest,
			*ResetRequest,
			*driverutil.ConfigurationHistoryRequest,
			*HistoryResponse,
		](
			options.DefaultConfigStore,
			options.ActiveConfigStore,
			flagutil.LoadDefaults,
		),
	}
}

// DryRun implements ExampleDriver.
func (d *DriverImpl) DryRun(ctx context.Context, req *DryRunRequest) (*DryRunResponse, error) {
	// The tracker implements all the logic for DryRun except for validation.
	results, err := d.Tracker().DryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &DryRunResponse{
		Current:          results.Current,
		Modified:         results.Modified,
		ValidationErrors: nil, // This is left as an implementation detail.
	}, nil
}

// Enables the history terminal UI
func (h *HistoryResponse) RenderText(out cli.Writer) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		out.Println(driverutil.MarshalConfigJson(h))
		return
	}
	ui := tui.NewHistoryUI(h.GetEntries())
	ui.Run()
}

func init() {
	// Adds the rollback command
	addExtraConfigCmd(rollback.BuildCmd("rollback", ConfigClientFromContext))

	drivers.Register("example", func(ctx context.Context, opts ...driverutil.Option) (ExampleDriver, error) {
		options := ExampleDriverImplOptions{}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}
		return NewExampleDriverImpl(options), nil
	})
}

type ConfigServerBackend struct {
	util.Initializer
	UnsafeConfigServer

	driver ExampleDriver
}

func (b *ConfigServerBackend) Initialize(driver ExampleDriver) {
	b.InitOnce(func() {
		b.driver = driver
	})
}

func (b *ConfigServerBackend) ConfigurationHistory(ctx context.Context, in *driverutil.ConfigurationHistoryRequest) (*HistoryResponse, error) {
	b.WaitForInit()
	return b.driver.ConfigurationHistory(ctx, in)
}

func (b *ConfigServerBackend) DryRun(ctx context.Context, in *DryRunRequest) (*DryRunResponse, error) {
	b.WaitForInit()
	return b.driver.DryRun(ctx, in)
}

func (b *ConfigServerBackend) GetConfiguration(ctx context.Context, in *driverutil.GetRequest) (*ConfigSpec, error) {
	b.WaitForInit()
	return b.driver.GetConfiguration(ctx, in)
}

func (b *ConfigServerBackend) GetDefaultConfiguration(ctx context.Context, in *driverutil.GetRequest) (*ConfigSpec, error) {
	b.WaitForInit()
	return b.driver.GetDefaultConfiguration(ctx, in)
}

func (b *ConfigServerBackend) ResetConfiguration(ctx context.Context, in *ResetRequest) (*emptypb.Empty, error) {
	b.WaitForInit()
	return b.driver.ResetConfiguration(ctx, in)
}

func (b *ConfigServerBackend) ResetDefaultConfiguration(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	b.WaitForInit()
	return b.driver.ResetDefaultConfiguration(ctx, in)
}

func (b *ConfigServerBackend) SetConfiguration(ctx context.Context, in *SetRequest) (*emptypb.Empty, error) {
	b.WaitForInit()
	return b.driver.SetConfiguration(ctx, in)
}

func (b *ConfigServerBackend) SetDefaultConfiguration(ctx context.Context, in *SetRequest) (*emptypb.Empty, error) {
	b.WaitForInit()
	return b.driver.SetDefaultConfiguration(ctx, in)
}

var _ ConfigServer = (*ConfigServerBackend)(nil)
