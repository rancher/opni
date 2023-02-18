package interfaces

/*
Import only module for alerting plugin - alerting apis need to implement
the interfaces defined in this module
*/

import (
	"google.golang.org/protobuf/proto"
)

type Cloneable[T any] interface {
	Clone() T
}

type AlertingSecret interface {
	proto.Message
	// redacts secret fields
	RedactSecrets()
}
