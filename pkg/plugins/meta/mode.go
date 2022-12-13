package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const (
	PluginModeEnvVar = "OPNI_PLUGIN_MODE"
)

type PluginMode string

const (
	ModeGateway   PluginMode = "gateway"
	ModeAgent     PluginMode = "agent"
	ModeListModes PluginMode = "__list_modes__"
)

func (m PluginMode) IsValid() bool {
	return m == ModeGateway || m == ModeAgent || m == ModeListModes
}

type ModeSet map[PluginMode]SchemeFunc

type ModeList struct {
	Modes []PluginMode `json:"modes"`
}

func (m ModeSet) MarshalJSON() ([]byte, error) {
	var modes []PluginMode
	for mode := range m {
		if mode == ModeListModes {
			continue
		}
		modes = append(modes, mode)
	}
	return json.Marshal(ModeList{
		Modes: modes,
	})
}

func QueryPluginModes(pluginFilename string) (ModeList, error) {
	outBuf := new(bytes.Buffer)
	ctx, ca := context.WithTimeout(context.Background(), 1*time.Second)
	defer ca()
	cmd := exec.CommandContext(ctx, pluginFilename)
	cmd.Env = []string{fmt.Sprintf("%s=%s", PluginModeEnvVar, ModeListModes)}
	cmd.Stdout = outBuf
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		return ModeList{}, err
	}
	var info ModeList
	if err := json.NewDecoder(outBuf).Decode(&info); err != nil {
		return ModeList{}, err
	}
	return info, nil
}
