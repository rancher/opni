package driverutil

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Driver any

type InstallableConfigType[T any] interface {
	ConfigType[T]
	GetEnabled() bool
}

type Revisioner interface {
	proto.Message
	GetRevision() *corev1.Revision
}

type SecretsRedactor[T any] interface {
	RedactSecrets()
	UnredactSecrets(T) error
}

type ConfigType[T any] interface {
	proto.Message
	Revisioner
	SecretsRedactor[T]
}

func WithNoopSecretsRedactor[U Revisioner, T any](partial U) ConfigType[T] {
	return struct {
		Revisioner
		SecretsRedactor[T]
	}{partial, NoopSecretsRedactor[T]{}}
}

type NoopSecretsRedactor[T any] struct{}

func (NoopSecretsRedactor[T]) RedactSecrets() {}

func (NoopSecretsRedactor[T]) UnredactSecrets(T) error { return nil }

type DryRunRequestType[T ConfigType[T]] interface {
	proto.Message
	GetAction() Action
	GetTarget() Target
	GetSpec() T
}

type MutableDryRunRequestType[T ConfigType[T]] interface {
	DryRunRequestType[T]
}

type DryRunResponseType[T ConfigType[T]] interface {
	proto.Message
	GetCurrent() T
	GetModified() T
	GetValidationErrors() []*ValidationError
}

type HistoryResponseType[T ConfigType[T]] interface {
	proto.Message
	GetEntries() []T
}

type ClientInterface[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
	HR HistoryResponseType[T],
] interface {
	GetDefaultConfiguration(context.Context, *GetRequest, ...grpc.CallOption) (T, error)
	SetDefaultConfiguration(context.Context, T, ...grpc.CallOption) (*emptypb.Empty, error)

	GetConfiguration(context.Context, *GetRequest, ...grpc.CallOption) (T, error)
	SetConfiguration(context.Context, T, ...grpc.CallOption) (*emptypb.Empty, error)

	DryRun(context.Context, D, ...grpc.CallOption) (DR, error)
	ConfigurationHistory(context.Context, *ConfigurationHistoryRequest, ...grpc.CallOption) (HR, error)
}

func AsClientInterface[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
	HR HistoryResponseType[T],
](client ClientInterface[T, D, DR, HR]) ClientInterface[T, D, DR, HR] {
	return client
}

type ServerInterface[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
] interface {
	GetDefaultConfiguration(context.Context, *GetRequest) (T, error)
	SetDefaultConfiguration(context.Context, T) (*emptypb.Empty, error)

	GetConfiguration(context.Context, *GetRequest) (T, error)
	SetConfiguration(context.Context, T) (*emptypb.Empty, error)

	DryRun(context.Context, D) (DR, error)
	ConfigurationHistory(context.Context)
}

func AsServerInterface[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
](server ServerInterface[T, D, DR]) ServerInterface[T, D, DR] {
	return server
}
