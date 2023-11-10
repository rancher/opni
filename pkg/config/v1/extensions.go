package v1

import (
	"os"

	cli "github.com/rancher/opni/internal/codegen/cli"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/dryrun"
	"github.com/rancher/opni/pkg/plugins/driverutil/rollback"
	"github.com/rancher/opni/pkg/tui"
	"golang.org/x/term"
)

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
	addExtraGatewayConfigCmd(rollback.BuildCmd("rollback", GatewayConfigContextInjector))
	addExtraGatewayConfigCmd(dryrun.BuildCmd("dry-run", GatewayConfigContextInjector))
}
