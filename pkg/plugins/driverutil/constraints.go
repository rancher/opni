package driverutil

import (
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/proto"
)

type Driver any

type InstallableConfigType[T any] interface {
	ConfigType[T]
	GetEnabled() bool
}

type RevisionGetter interface {
	GetRevision() *corev1.Revision
}

type SecretsRedactor[T any] interface {
	RedactSecrets()
	UnredactSecrets(T) error
}

type ConfigType[T any] interface {
	proto.Message
	GetRevision() *corev1.Revision
	SecretsRedactor[T]
}
