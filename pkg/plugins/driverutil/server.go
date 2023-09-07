package driverutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type DefaultConfigurableServer[T InstallableConfigType[T], GR Revisioner] interface {
	GetDefaultConfiguration(context.Context, GR) (T, error)
	SetDefaultConfiguration(context.Context, T) (*emptypb.Empty, error)
	ResetDefaultConfiguration(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	GetConfiguration(context.Context, GR) (T, error)
	SetConfiguration(context.Context, T) (*emptypb.Empty, error)
	ResetConfiguration(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	Install(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	Uninstall(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

// Implements a subset of methods usually required by a driver which uses a DefaultingConfigTracker
// to manage its configuration. These implementations should not vary between drivers, so they are
// provided here as a convenience.
//
// Usage example:
//
//	type Driver struct {
//		foo.UnsafeFooServer
//
//		// package foo;
//		// message GetRequest {
//		//   core.Revision revision = 1;
//		// }
//		driverutil.ConfigurableServerInterface[T, *foo.GetRequest]
//	}
//
//	func NewDriver() *Driver {
//		tracker := driverutil.NewDefaultingConfigTracker[T](...)
//		return &Driver{
//			ConfigurableServerInterface: driverutil.NewDefaultConfigurableServer[foo.FooServer](tracker),
//		}
//	}
func NewDefaultConfigurableServer[
	I DefaultConfigurableServer[T, GR],
	T InstallableConfigType[T],
	GR Revisioner,
](tracker *DefaultingConfigTracker[T]) DefaultConfigurableServer[T, GR] {
	return &defaultConfigurableServer[T, GR]{
		tracker:             tracker,
		indexOfEnabledField: getEnabledFieldIndex[T](),
	}
}

type defaultConfigurableServer[T InstallableConfigType[T], GR Revisioner] struct {
	tracker             *DefaultingConfigTracker[T]
	indexOfEnabledField protowire.Number
}

// GetConfiguration implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) GetConfiguration(ctx context.Context, in GR) (T, error) {
	return d.tracker.GetConfigOrDefault(ctx, in.GetRevision())

}

// GetDefaultConfiguration implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) GetDefaultConfiguration(ctx context.Context, in GR) (T, error) {
	return d.tracker.GetDefaultConfig(ctx, in.GetRevision())
}

// Install implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) Install(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	var t T
	t = t.ProtoReflect().New().Interface().(T)
	d.setEnabled(t, lo.ToPtr(true))
	err := d.tracker.ApplyConfig(ctx, t)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to install monitoring cluster: %s", err.Error())
	}
	return &emptypb.Empty{}, nil
}

// ResetDefaultConfiguration implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) ResetDefaultConfiguration(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if err := d.tracker.ResetDefaultConfig(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ResetConfiguration implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) ResetConfiguration(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	var t T
	enabledFieldName := t.ProtoReflect().Descriptor().Fields().ByNumber(protowire.Number(d.indexOfEnabledField)).Name()
	mask := &fieldmaskpb.FieldMask{
		Paths: []string{string(enabledFieldName)},
	}
	if err := d.tracker.ResetConfig(ctx, mask); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// SetConfiguration implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) SetConfiguration(ctx context.Context, t T) (*emptypb.Empty, error) {
	d.setEnabled(t, nil)
	if err := d.tracker.ApplyConfig(ctx, t); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// SetDefaultConfiguration implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) SetDefaultConfiguration(ctx context.Context, t T) (*emptypb.Empty, error) {
	d.setEnabled(t, nil)
	if err := d.tracker.SetDefaultConfig(ctx, t); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Uninstall implements ConfigurableServerInterface.
func (d *defaultConfigurableServer[T, GR]) Uninstall(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	var t T
	t = t.ProtoReflect().New().Interface().(T)
	d.setEnabled(t, lo.ToPtr(false))
	err := d.tracker.ApplyConfig(ctx, t)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to uninstall monitoring cluster: %s", err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (d *defaultConfigurableServer[T, GR]) setEnabled(t T, enabled *bool) T {
	field := t.ProtoReflect().Descriptor().Fields().ByNumber(d.indexOfEnabledField)
	msg := t.ProtoReflect()
	if msg.Has(field) && enabled == nil {
		msg.Clear(field)
	} else if enabled != nil {
		msg.Set(field, protoreflect.ValueOfBool(*enabled))
	}
	return t
}

func getEnabledFieldIndex[T ConfigType[T]]() protowire.Number {
	var t T
	fields := t.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if strings.ToLower(string(field.Name())) == "enabled" && field.HasPresence() && field.Kind() == protoreflect.BoolKind {
			return field.Number()
		}
	}
	panic(fmt.Sprintf("field not found in type %T: `optional bool enabled`", t))
}
