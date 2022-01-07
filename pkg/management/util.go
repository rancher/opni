package management

import (
	"os"
	"path/filepath"
)

func DefaultManagementSocket() string {
	// check if we're root
	if os.Geteuid() == 0 {
		return "unix:///run/opni-monitoring/management.sock"
	}
	// check if $XDG_RUNTIME_DIR is set
	if runUser, ok := os.LookupEnv("XDG_RUNTIME_DIR"); ok {
		return "unix://" + filepath.Join(runUser, "opni-monitoring/management.sock")
	}
	return "unix:///tmp/opni-monitoring/management.sock"
}
