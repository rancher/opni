package system

import (
	"context"
	"fmt"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ExtensionClientInterface allows plugins to obtain clients to API extensions
// implemented by other plugins.
type ExtensionClientInterface interface {
	// Attempt to obtain a ClientConn such that the API extensions with the given
	// names will be available on that client. If the requested extension is not
	// available, it will retry until it becomes available, or until the context
	// is canceled.
	GetClientConn(ctx context.Context, serviceNames ...string) (grpc.ClientConnInterface, error)

	// Returns a ClientConn that makes no guarantees about the availability of
	// any API extensions.
	GetClientConnUnchecked() grpc.ClientConnInterface
}

type apiExtensionInterfaceImpl struct {
	managementClientConn *grpc.ClientConn
}

func (c *apiExtensionInterfaceImpl) GetClientConn(ctx context.Context, serviceNames ...string) (grpc.ClientConnInterface, error) {
	// Use c.managementClientConn to obtain a management client, then query the api extension list for that name.
	// Only when the extension becomes available, return c.managementClientConn.
	client := managementv1.NewManagementClient(c.managementClientConn)
	apiExtensions, err := client.APIExtensions(ctx, &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		return nil, err
	}
EXTENSIONS:
	for _, name := range serviceNames {
		for _, ext := range apiExtensions.Items {
			if ext.ServiceDesc.GetName() == name {
				continue EXTENSIONS
			}
		}
		fmt.Println("available extensions:")
		for _, ext := range apiExtensions.Items {
			fmt.Println(ext.ServiceDesc.GetName())
		}
		return nil, fmt.Errorf("api extension %s is not available", name)
	}
	return c.managementClientConn, nil
}

func (c *apiExtensionInterfaceImpl) GetClientConnUnchecked() grpc.ClientConnInterface {
	return c.managementClientConn
}
