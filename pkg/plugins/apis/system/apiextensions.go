package system

import (
	"context"
	"fmt"
	"time"

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
}

type apiExtensionInterfaceImpl struct {
	managementClientConn *grpc.ClientConn
}

func (c *apiExtensionInterfaceImpl) GetClientConn(ctx context.Context, serviceNames ...string) (grpc.ClientConnInterface, error) {
	//FIXME:(alex) Not being called
	found := false
	serviceMap := make(map[string]bool)
	discovered := make(map[string]bool)
	for k := range serviceNames {
		serviceMap[serviceNames[k]] = true
		discovered[serviceNames[k]] = false
	}
	// Use c.managementClientConn to obtain a management client, then query the api extension list for that name.
	// Only when the extension becomes available, return c.managementClientConn.
	client := managementv1.NewManagementClient(c.managementClientConn)
	for retries := 10; retries > 0; retries-- {
		apiExtensions, err := client.APIExtensions(ctx, &emptypb.Empty{})
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		for _, ext := range apiExtensions.Items {
			if serviceMap[ext.ServiceDesc.GetName()] {
				discovered[ext.ServiceDesc.GetName()] = true
			}
		}
		for _, has := range discovered {
			found = found && has
		}

		if found {
			break
		} else {
			time.Sleep(time.Second)
		}
	}
	if found {
		return nil, fmt.Errorf("Failed to get one or more API extensions")
	}
	return c.managementClientConn, nil
}
