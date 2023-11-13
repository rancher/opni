package pathreflect

import (
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type RootPathBuilder[T ~[]protopath.Step] interface {
	proto.Message
	ProtoPath() T
}

// Returns a list of all possible paths from the given root path builder.
func AllPaths[T RootPathBuilder[U], U ~[]protopath.Step](root T) []protopath.Path {
	rootPath := root.ProtoPath()
	rootType := reflect.TypeOf(rootPath)
	chains := buildAllMethodChains(rootType)
	return resolveMethodChains(chains, reflect.ValueOf(rootPath))
}

// Returns the value at the given path in the message.
// This only supports the step kinds implemented by code generated from the
// path builder generator.
func Value[T RootPathBuilder[U], U ~[]protopath.Step](msg T, path protopath.Path) protoreflect.Value {
	value := protoreflect.ValueOfMessage(msg.ProtoReflect())
	for _, step := range path {
		switch step.Kind() {
		case protopath.RootStep:
			continue
		case protopath.FieldAccessStep:
			value = value.Message().Get(step.FieldDescriptor())
		default:
			panic("unsupported step kind: " + step.Kind().String())
		}
	}
	return value
}

var typeOfProtopathPath = reflect.TypeOf((*protopath.Path)(nil)).Elem()

// builds a list of all possible method chains from the root path builder
func buildAllMethodChains(root reflect.Type) [][]reflect.Value {
	chains := [][]reflect.Value{}

	// Iterate over methods
	for i := 0; i < root.NumMethod(); i++ {
		mtd := root.Method(i)

		// Check if the method returns a protopath.Path
		if mtd.Type.NumOut() == 1 && mtd.Type.Out(0).ConvertibleTo(typeOfProtopathPath) {
			methodChain := []reflect.Value{mtd.Func}
			receiverType := mtd.Type.Out(0)

			// Recursively build chains from this type
			subChains := buildAllMethodChains(receiverType)
			chains = append(chains, methodChain)

			for _, chain := range subChains {
				chains = append(chains, append(methodChain, chain...))
			}

		}
	}
	return chains
}

func resolveMethodChains(chains [][]reflect.Value, root reflect.Value) []protopath.Path {
	paths := []protopath.Path{}
	for _, chain := range chains {
		receiver := root
		for _, mtd := range chain {
			// Use the return value of the previous method as the next receiver
			receiver = mtd.Call([]reflect.Value{receiver})[0]
		}
		paths = append(paths, receiver.Convert(typeOfProtopathPath).Interface().(protopath.Path))
	}
	return paths
}
