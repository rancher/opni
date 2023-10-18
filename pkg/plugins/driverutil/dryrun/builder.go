package dryrun

import (
	"reflect"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func NewDryRunRequest[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
]() DryRunRequestBuilder[T, D] {
	return &dryRunRequestBuilderImpl[T, D]{
		request: util.NewMessage[D](),
	}
}

type DryRunRequestBuilder[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	Default() DryRunRequestBuilder_Default[T, D]
	Active() DryRunRequestBuilder_Active[T, D]
}

type DryRunRequestBuilder_Active[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	Set() DryRunRequestBuilder_Set[T, D]
	Reset() DryRunRequestBuilder_Reset[T, D]
}

type DryRunRequestBuilder_Default[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	Set() DryRunRequestBuilder_Set[T, D]
	Reset() DryRunRequestBuilder_Build[T, D]
}

type DryRunRequestBuilder_Set[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	Spec(spec T) DryRunRequestBuilder_Build[T, D]
}

type DryRunRequestBuilder_Reset[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	Revision(rev *corev1.Revision) DryRunRequestBuilder_ResetOrBuild[T, D]
	Patch(patch T) DryRunRequestBuilder_ResetOrBuild[T, D]
	Mask(mask *fieldmaskpb.FieldMask) DryRunRequestBuilder_ResetOrBuild[T, D]
}

type DryRunRequestBuilder_Build[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	Build() D
}

type DryRunRequestBuilder_ResetOrBuild[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] interface {
	DryRunRequestBuilder_Reset[T, D]
	DryRunRequestBuilder_Build[T, D]
}

type dryRunRequestBuilderImpl[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] struct {
	request D
}

type dryRunRequestBuilderImpl_ResetDefault[
	T driverutil.ConfigType[T],
	D driverutil.DryRunRequestType[T],
] struct {
	*dryRunRequestBuilderImpl[T, D]
}

func (b dryRunRequestBuilderImpl_ResetDefault[T, D]) Reset() DryRunRequestBuilder_Build[T, D] {
	b.dryRunRequestBuilderImpl.Reset()
	return b.dryRunRequestBuilderImpl
}

func (b *dryRunRequestBuilderImpl[T, D]) Mask(mask *fieldmaskpb.FieldMask) DryRunRequestBuilder_ResetOrBuild[T, D] {
	if mask != nil {
		b.request.ProtoReflect().Set(util.FieldByName[D]("mask"), protoreflect.ValueOf(mask))
	}
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Patch(patch T) DryRunRequestBuilder_ResetOrBuild[T, D] {
	if !reflect.ValueOf(patch).IsNil() {
		b.request.ProtoReflect().Set(util.FieldByName[D]("patch"), protoreflect.ValueOf(patch))
	}
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Revision(rev *corev1.Revision) DryRunRequestBuilder_ResetOrBuild[T, D] {
	if rev != nil {
		b.request.ProtoReflect().Set(util.FieldByName[D]("revision"), protoreflect.ValueOf(rev))
	}
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Set() DryRunRequestBuilder_Set[T, D] {
	b.request.ProtoReflect().Set(util.FieldByName[D]("action"), protoreflect.ValueOfEnum(driverutil.Action_Set.Number()))
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Reset() DryRunRequestBuilder_Reset[T, D] {
	b.request.ProtoReflect().Set(util.FieldByName[D]("action"), protoreflect.ValueOfEnum(driverutil.Action_Reset.Number()))
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Default() DryRunRequestBuilder_Default[T, D] {
	b.request.ProtoReflect().Set(util.FieldByName[D]("target"), protoreflect.ValueOfEnum(driverutil.Target_DefaultConfiguration.Number()))
	return dryRunRequestBuilderImpl_ResetDefault[T, D]{b}
}

func (b *dryRunRequestBuilderImpl[T, D]) Active() DryRunRequestBuilder_Active[T, D] {
	b.request.ProtoReflect().Set(util.FieldByName[D]("target"), protoreflect.ValueOfEnum(driverutil.Target_ActiveConfiguration.Number()))
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Spec(spec T) DryRunRequestBuilder_Build[T, D] {
	b.request.ProtoReflect().Set(util.FieldByName[D]("spec"), protoreflect.ValueOf(spec))
	return b
}

func (b *dryRunRequestBuilderImpl[T, D]) Build() D {
	return b.request
}
