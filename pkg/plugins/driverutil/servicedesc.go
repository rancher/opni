package driverutil

import (
	"fmt"

	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func LoadServiceDescriptor(sp util.ServicePackInterface) (protoreflect.ServiceDescriptor, *descriptorpb.ServiceDescriptorProto, error) {
	desc, _ := sp.Unpack()
	svcFqn := protoreflect.FullName(desc.ServiceName)
	if !svcFqn.IsValid() {
		return nil, nil, fmt.Errorf("invalid service name %s in file %s", desc.ServiceName, desc.Metadata)
	}
	fdp, err := protoregistry.GlobalFiles.FindFileByPath(desc.Metadata.(string))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve file descriptor: %w", err)
	}

	descriptor := fdp.Services().ByName(svcFqn.Name())
	if descriptor == nil {
		return nil, nil, fmt.Errorf("failed to find service descriptor %s in file %s", desc.ServiceName, desc.Metadata)
	}
	dpb := protodesc.ToServiceDescriptorProto(descriptor)
	*dpb.Name = string(descriptor.FullName())
	return descriptor, dpb, nil
}
