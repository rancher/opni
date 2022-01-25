package management

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/kralicky/opni-monitoring/pkg/storage"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
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

func grpcError(err error) error {
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return status.Error(codes.NotFound, err.Error())
		}
		return status.Error(codes.Internal, err.Error())
	}
	return nil
}
