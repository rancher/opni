package cliutil

import "google.golang.org/protobuf/proto"

func InitializeField[T proto.Message](pt *T) {
	*pt = (*pt).ProtoReflect().New().Interface().(T)
}
