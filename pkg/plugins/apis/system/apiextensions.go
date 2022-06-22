package system

import (
	"context"

	"google.golang.org/grpc"
)

// ExtensionClientInterface allows plugins to obtain clients to API extensions
// implemented by other plugins.
type ExtensionClientInterface interface {
	// Attempt to obtain a ClientConn such that the API extensions with the given
	// names will be available on that client. If the requested extension is not
	// available, it will retry until it becomes available, or until the context
	// is canceled.
	GetClientConn(ctx context.Context, serviceNames ...string) (grpc.ClientConnInterface, error)
}

type apiExtensionInterfaceImpl struct {
	managementClientConn *grpc.ClientConn
}

func (c *apiExtensionInterfaceImpl) GetClientConn(ctx context.Context, serviceNames ...string) (grpc.ClientConnInterface, error) {
	// TODO(alex): implement this function
	// Use c.managementClientConn to obtain a management client, then query the api extension list for that name.
	// Only when the extension becomes available, return c.managementClientConn.
	return nil, nil
}
