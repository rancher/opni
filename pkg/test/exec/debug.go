package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"

	"github.com/rancher/opni/pkg/test/freeport"
)

var (
	DlvProcesses = []DebugProcess{}
	DlvMutex     = sync.Mutex{}
)

type DebugProcess struct {
	Port   int
	Binary string
	Pid    int
	Error  error
}

func (d *DebugProcess) Key() string {
	return fmt.Sprintf("%s:%d", d.Binary, d.Port)
}

func ListDebugProcesses() []DebugProcess {
	copy := make([]DebugProcess, len(DlvProcesses))
	DlvMutex.Lock()
	defer DlvMutex.Unlock()
	for i := range DlvProcesses {
		copy[i] = DlvProcesses[i]
	}
	return copy
}

func DebugCommandContext(ctx context.Context, binary string, args ...string) *exec.Cmd {
	dlvBin := "dlv"
	if os.Getenv("GOBIN") != "" {
		dlvBin = path.Join(os.Getenv("GOBIN"), "dlv")
	}
	dlvPort := freeport.GetFreePort()
	debugArgs := append([]string{
		"exec", "--headless", fmt.Sprintf("--listen=:%d", dlvPort), "--api-version=2", "--accept-multiclient", "--log",
		binary,
		"--"},
		args...,
	)
	DlvMutex.Lock()
	DlvProcesses = append(DlvProcesses, DebugProcess{
		Port:   dlvPort,
		Binary: binary,
		Pid:    -1,
		Error:  nil,
	})
	DlvMutex.Unlock()
	return exec.CommandContext(ctx, dlvBin, debugArgs...)
}
