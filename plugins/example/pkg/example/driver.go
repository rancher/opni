package example

import (
	context "context"
	"os"

	cli "github.com/rancher/opni/internal/codegen/cli"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/rollback"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tui"
	flagutil "github.com/rancher/opni/pkg/util/flagutil"
	"golang.org/x/term"
)

type ExampleDriver struct {
	*driverutil.BaseConfigServer[
		*driverutil.GetRequest,
		*SetRequest,
		*ResetRequest,
		*driverutil.ConfigurationHistoryRequest,
		*HistoryResponse,
		*ConfigSpec,
	]
}

type ExampleDriverImplOptions struct {
	DefaultConfigStore storage.ValueStoreT[*ConfigSpec]
	ActiveConfigStore  storage.ValueStoreT[*ConfigSpec]
}

func (d *ExampleDriver) Initialize(options ExampleDriverImplOptions) {
	d.BaseConfigServer = d.BaseConfigServer.Build(
		options.DefaultConfigStore,
		options.ActiveConfigStore,
		flagutil.LoadDefaults,
	)
}

// DryRun implements ExampleDriver.
func (d *ExampleDriver) DryRun(ctx context.Context, req *DryRunRequest) (*DryRunResponse, error) {
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
}
