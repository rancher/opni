package driverutil

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	flagutil "github.com/rancher/opni/pkg/util/flagutil"
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

// Default constraint for a Get request.
// Not generic; the built-in message type [driverutil.GetRequest] can be used for convenience
type GetRequestType interface {
	proto.Message
	flagutil.FlagSetter
	GetRevision() *corev1.Revision
}

type SetRequestType[T ConfigType[T]] interface {
	proto.Message
	GetSpec() T
}

// Default constraint for a History request.
// Not generic; the built-in message type [driverutil.ConfigurationHistoryRequest] can be used for convenience
type HistoryRequestType interface {
	proto.Message
	flagutil.FlagSetter
	GetTarget() Target
	GetRevision() *corev1.Revision
	GetIncludeValues() bool
}

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

type BasicServer[
	T ConfigType[T],
	G GetRequestType,
	S SetRequestType[T],
] interface {
	BasicDefaultServer[T, G, S]
	BasicActiveServer[T, G, S]
}

type BasicDefaultServer[
	T ConfigType[T],
	G GetRequestType,
	S SetRequestType[T],
] interface {
	GetDefaultConfiguration(context.Context, G) (T, error)
	SetDefaultConfiguration(context.Context, S) (*emptypb.Empty, error)
}

type BasicActiveServer[
	T ConfigType[T],
	G GetRequestType,
	S SetRequestType[T],
] interface {
	GetConfiguration(context.Context, G) (T, error)
	SetConfiguration(context.Context, S) (*emptypb.Empty, error)
}

type ResetServer[
	T ConfigType[T],
	R ResetRequestType[T],
] interface {
	ResetDefaultServer[T, R]
	ResetActiveServer[T, R]
}

type ResetDefaultServer[
	T ConfigType[T],
	R ResetRequestType[T],
] interface {
	ResetDefaultConfiguration(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

type ResetActiveServer[
	T ConfigType[T],
	R ResetRequestType[T],
] interface {
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
	H HistoryRequestType,
	HR HistoryResponseType[T],
] interface {
	ConfigurationHistory(context.Context, H) (HR, error)
}

type ConfigServer[
	T ConfigType[T],
	G GetRequestType,
	S SetRequestType[T],
	R ResetRequestType[T],
	H HistoryRequestType,
	HR HistoryResponseType[T],
] interface {
	BasicServer[T, G, S]
	ResetServer[T, R]
	HistoryServer[T, H, HR]
}

type DryRunConfigServer[
	T ConfigType[T],
	G GetRequestType,
	S SetRequestType[T],
	R ResetRequestType[T],
	H HistoryRequestType,
	HR HistoryResponseType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
] interface {
	ConfigServer[T, G, S, R, H, HR]
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

type GetClient[
	T ConfigType[T],
	G GetRequestType,
] interface {
	GetDefaultConfiguration(context.Context, G, ...grpc.CallOption) (T, error)
	GetConfiguration(context.Context, G, ...grpc.CallOption) (T, error)
}

type SetClient[
	T ConfigType[T],
	S SetRequestType[T],
] interface {
	SetDefaultConfiguration(context.Context, S, ...grpc.CallOption) (*emptypb.Empty, error)
	SetConfiguration(context.Context, S, ...grpc.CallOption) (*emptypb.Empty, error)
}

type ResetClient[
	T ConfigType[T],
	R ResetRequestType[T],
] interface {
	ResetDefaultConfiguration(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error)
	ResetConfiguration(context.Context, R, ...grpc.CallOption) (*emptypb.Empty, error)
}

type DryRunClient[
	T ConfigType[T],
	D DryRunRequestType[T],
	DR DryRunResponseType[T],
] interface {
	DryRun(context.Context, D, ...grpc.CallOption) (DR, error)
}

type HistoryClient[
	T ConfigType[T],
	H HistoryRequestType,
	HR HistoryResponseType[T],
] interface {
	ConfigurationHistory(context.Context, H, ...grpc.CallOption) (HR, error)
}
