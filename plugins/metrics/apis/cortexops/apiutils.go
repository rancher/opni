package cortexops

import (
	context "context"
	"fmt"

	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func InstallWithPreset(ctx context.Context, client CortexOpsClient, presetId ...string) error {
	presetList, err := client.ListPresets(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	if len(presetList.Items) == 0 {
		return fmt.Errorf("no presets found")
	}
	var targetPreset *Preset
	if len(presetId) == 0 {
		targetPreset = presetList.Items[0]
	} else {
		for _, preset := range presetList.Items {
			if preset.GetId().GetId() == presetId[0] {
				targetPreset = preset
				break
			}
		}
		if targetPreset == nil {
			return fmt.Errorf("preset %s not found", presetId[0])
		}
	}
	if _, err := client.SetConfiguration(ctx, &SetRequest{
		Spec: targetPreset.GetSpec(),
	}); err != nil {
		return err
	}
	_, err = client.Install(ctx, &emptypb.Empty{})
	return err
}

func WaitForReady(ctx context.Context, client CortexOpsClient) error {
	var lastErr error
	for ctx.Err() == nil {
		var status *driverutil.InstallStatus
		status, lastErr = client.Status(ctx, &emptypb.Empty{})
		if lastErr != nil {
			continue
		}
		if status.AppState != driverutil.ApplicationState_Running {
			lastErr = fmt.Errorf("waiting for application state to be running; current state: %s", status.AppState.String())
			continue
		}
		break
	}

	return lastErr
}

type SpecializedConfigServer = driverutil.ConfigServer[
	*CapabilityBackendConfigSpec,
	*driverutil.GetRequest,
	*SetRequest,
	*ResetRequest,
	*driverutil.ConfigurationHistoryRequest,
	*ConfigurationHistoryResponse,
]

type SpecializedDryRunServer = driverutil.DryRunServer[
	*CapabilityBackendConfigSpec,
	*DryRunRequest,
	*DryRunResponse,
]
