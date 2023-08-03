package cortexops

import (
	context "context"
	"fmt"

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
	if _, err := client.SetConfiguration(ctx, targetPreset.GetSpec()); err != nil {
		return err
	}
	_, err = client.Install(ctx, &emptypb.Empty{})
	return err
}
