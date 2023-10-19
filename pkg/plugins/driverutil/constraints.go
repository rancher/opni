package driverutil

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
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

type ListType[T any] interface {
	proto.Message
	GetItems() []T
}

type PresetType[T any] interface {
	proto.Message
	GetId() *corev1.Reference
	GetMetadata() *PresetMetadata
	GetSpec() T
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

type ResetRequestType[T ConfigType[T]] interface {
	proto.Message
	GetMask() *fieldmaskpb.FieldMask
	GetPatch() T
}

type DryRunRequestType[
	T ConfigType[T],
] interface {
	proto.Message
	GetAction() Action
	GetTarget() Target
	GetSpec() T
	GetRevision() *corev1.Revision
	GetPatch() T
	GetMask() *fieldmaskpb.FieldMask
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

type BasicClientInterface[
	T ConfigType[T],
	R ResetRequestType[T],
] interface {
	GetDefaultConfiguration(context.Context, *GetRequest, ...grpc.CallOption) (T, error)
	SetDefaultConfiguration(context.Context, T, ...grpc.CallOption) (*emptypb.Empty, error)
	ResetDefaultConfiguration(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error)
	GetConfiguration(context.Context, *GetRequest, ...grpc.CallOption) (T, error)
	SetConfiguration(context.Context, T, ...grpc.CallOption) (*emptypb.Empty, error)
	ResetConfiguration(context.Context, R, ...grpc.CallOption) (*emptypb.Empty, error)
}

type Client[
	T ConfigType[T],
	R ResetRequestType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
	HR HistoryResponseType[T],
] interface {
	BasicClientInterface[T, R]
	DryRunClientInterface[T, D, DR]
	HistoryClientInterface[T, HR]
}

type DryRunClientInterface[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
] interface {
	DryRun(context.Context, D, ...grpc.CallOption) (DR, error)
}

type HistoryClientInterface[
	T ConfigType[T],
	HR HistoryResponseType[T],
] interface {
	ConfigurationHistory(context.Context, *ConfigurationHistoryRequest, ...grpc.CallOption) (HR, error)
}

type BasicServer[
	T ConfigType[T],
] interface {
	GetDefaultConfiguration(context.Context, *GetRequest) (T, error)
	SetDefaultConfiguration(context.Context, T) (*emptypb.Empty, error)
	GetConfiguration(context.Context, *GetRequest) (T, error)
	SetConfiguration(context.Context, T) (*emptypb.Empty, error)
}

type ResetServer[
	T ConfigType[T],
	R ResetRequestType[T],
] interface {
	ResetDefaultConfiguration(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	ResetConfiguration(context.Context, R) (*emptypb.Empty, error)
}

type DryRunServer[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
] interface {
	DryRun(context.Context, D) (DR, error)
}

type HistoryServer[
	T ConfigType[T],
	HR HistoryResponseType[T],
] interface {
	ConfigurationHistory(context.Context, *ConfigurationHistoryRequest) (HR, error)
}

type ConfigServer[
	T ConfigType[T],
	R ResetRequestType[T],
	HR HistoryResponseType[T],
] interface {
	BasicServer[T]
	ResetServer[T, R]
	HistoryServer[T, HR]
}

type DryRunConfigServer[
	T ConfigType[T],
	R ResetRequestType[T],
	HR HistoryResponseType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
] interface {
	ConfigServer[T, R, HR]
	DryRunServer[T, D, DR]
}

type InstallerServer interface {
	Install(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	Uninstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	Status(context.Context, *emptypb.Empty) (*InstallStatus, error)
}

type InstallerClient interface {
	Install(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error)
	Uninstall(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error)
	Status(context.Context, *emptypb.Empty, ...grpc.CallOption) (*InstallStatus, error)
}
