package driverutil

import (
	"context"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// Implements a subset of methods usually required by a server which uses a DefaultingConfigTracker
// to manage its configuration. These implementations should not vary between drivers, so they are
// provided here as a convenience.
type BaseConfigServer[
	G GetRequestType,
	S SetRequestType[T],
	R ResetRequestType[T],
	H HistoryRequestType,
	HR HistoryResponseType[T],
	T ConfigType[T],
] struct {
	tracker *DefaultingConfigTracker[T]
}

// Returns a new instance of the BaseConfigServer with the defined type.
// This is a conveience function to avoid repeating the type parameters.
func (BaseConfigServer[G, S, R, H, HR, T]) New(
	defaultStore, activeStore storage.ValueStoreT[T],
	loadDefaultsFunc DefaultLoaderFunc[T],
) *BaseConfigServer[G, S, R, H, HR, T] {
	return &BaseConfigServer[G, S, R, H, HR, T]{
		tracker: NewDefaultingConfigTracker[T](defaultStore, activeStore, loadDefaultsFunc),
	}
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) GetConfiguration(ctx context.Context, in G) (T, error) {
	return s.tracker.GetConfigOrDefault(ctx, in.GetRevision())
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) GetDefaultConfiguration(ctx context.Context, in G) (T, error) {
	return s.tracker.GetDefaultConfig(ctx, in.GetRevision())
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) Install(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	var t T
	t = t.ProtoReflect().New().Interface().(T)
	s.setEnabled(t, lo.ToPtr(true))
	err := s.tracker.ApplyConfig(ctx, t)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to install monitoring cluster: %s", err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) ResetDefaultConfiguration(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if err := s.tracker.ResetDefaultConfig(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) ResetConfiguration(ctx context.Context, in R) (*emptypb.Empty, error) {
	// If T contains a field named "enabled", assume it has installation semantics
	// and ensure a non-nil mask is always passed to ResetConfig. This ensures
	// the active config is never deleted from the underlying store, and therefore
	// history is always preserved.
	if enabledField := util.FieldByName[T]("enabled"); enabledField != nil {
		if in.GetMask() == nil {
			in.ProtoReflect().Set(util.FieldByName[R]("mask"), protoreflect.ValueOfMessage(util.NewMessage[*fieldmaskpb.FieldMask]().ProtoReflect()))
		}
		var t T
		if err := in.GetMask().Append(t, "enabled"); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid mask: %s", err.Error())
		}
		patch := in.GetPatch()
		if patch.ProtoReflect().Has(enabledField) {
			// ensure the enabled field cannot be modified by the patch
			patch.ProtoReflect().Clear(enabledField)
		}
	}
	if err := s.tracker.ResetConfig(ctx, in.GetMask(), in.GetPatch()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) SetConfiguration(ctx context.Context, in S) (*emptypb.Empty, error) {
	s.setEnabled(in.GetSpec(), nil)
	if err := s.tracker.ApplyConfig(ctx, in.GetSpec()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) SetDefaultConfiguration(ctx context.Context, in S) (*emptypb.Empty, error) {
	s.setEnabled(in.GetSpec(), nil)
	if err := s.tracker.SetDefaultConfig(ctx, in.GetSpec()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) Uninstall(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	var t T
	t = t.ProtoReflect().New().Interface().(T)
	s.setEnabled(t, lo.ToPtr(false))
	err := s.tracker.ApplyConfig(ctx, t)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to uninstall monitoring cluster: %s", err.Error())
	}
	return &emptypb.Empty{}, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) setEnabled(t T, enabled *bool) {
	field := util.FieldByName[T]("enabled")
	if field == nil {
		return
	}
	msg := t.ProtoReflect()
	if msg.Has(field) && enabled == nil {
		msg.Clear(field)
	} else if enabled != nil {
		msg.Set(field, protoreflect.ValueOfBool(*enabled))
	}
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) ConfigurationHistory(ctx context.Context, in H) (HR, error) {
	options := []storage.HistoryOpt{
		storage.IncludeValues(in.GetIncludeValues()),
	}
	if in.GetRevision() != nil {
		options = append(options, storage.WithRevision(in.GetRevision().GetRevision()))
	}
	revisions, err := s.tracker.History(ctx, in.GetTarget(), options...)
	resp := util.NewMessage[HR]()
	if err != nil {
		return resp, err
	}
	entries := resp.ProtoReflect().Mutable(util.FieldByName[HR]("entries")).List()
	for _, rev := range revisions {
		if in.GetIncludeValues() {
			spec := rev.Value()
			SetRevision(spec, rev.Revision(), rev.Timestamp())
			entries.Append(protoreflect.ValueOfMessage(spec.ProtoReflect()))
		} else {
			newSpec := util.NewMessage[T]()
			SetRevision(newSpec, rev.Revision(), rev.Timestamp())
			entries.Append(protoreflect.ValueOfMessage(newSpec.ProtoReflect()))
		}
	}
	return resp, nil
}

func (s *BaseConfigServer[G, S, R, H, HR, T]) Tracker() *DefaultingConfigTracker[T] {
	return s.tracker
}

type ContextKeyableConfigServer[
	G interface {
		GetRequestType
		ContextKeyable
	},
	S interface {
		SetRequestType[T]
		ContextKeyable
	},
	R interface {
		ResetRequestType[T]
		ContextKeyable
	},
	H interface {
		HistoryRequestType
		ContextKeyable
	},
	HR HistoryResponseType[T],
	T ConfigType[T],
] struct {
	base *BaseConfigServer[G, S, R, H, HR, T]
}

// Returns a new instance of the ContextKeyableConfigServer with the defined type.
// This is a conveience function to avoid repeating the type parameters.
func (ContextKeyableConfigServer[G, S, R, H, HR, T]) New(
	defaultStore storage.ValueStoreT[T],
	activeStore storage.KeyValueStoreT[T],
	loadDefaultsFunc DefaultLoaderFunc[T],
) *ContextKeyableConfigServer[G, S, R, H, HR, T] {
	tracker := NewDefaultingActiveKeyedConfigTracker(
		defaultStore,
		activeStore,
		loadDefaultsFunc,
	)
	return &ContextKeyableConfigServer[G, S, R, H, HR, T]{
		base: &BaseConfigServer[G, S, R, H, HR, T]{
			tracker: tracker,
		},
	}
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) GetDefaultConfiguration(ctx context.Context, in G) (T, error) {
	return s.base.GetDefaultConfiguration(ctx, in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) ResetDefaultConfiguration(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	return s.base.ResetDefaultConfiguration(ctx, in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) SetDefaultConfiguration(ctx context.Context, in S) (*emptypb.Empty, error) {
	return s.base.SetDefaultConfiguration(ctx, in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) GetConfiguration(ctx context.Context, in G) (T, error) {
	return s.base.GetConfiguration(contextWithKey(ctx, in.ContextKey()), in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) ResetConfiguration(ctx context.Context, in R) (*emptypb.Empty, error) {
	return s.base.ResetConfiguration(contextWithKey(ctx, in.ContextKey()), in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) SetConfiguration(ctx context.Context, in S) (*emptypb.Empty, error) {
	return s.base.SetConfiguration(contextWithKey(ctx, in.ContextKey()), in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) ConfigurationHistory(ctx context.Context, in H) (HR, error) {
	return s.base.ConfigurationHistory(contextWithKey(ctx, in.ContextKey()), in)
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) Install(ctx context.Context, in ContextKeyable) (*emptypb.Empty, error) {
	return s.base.Install(contextWithKey(ctx, in.ContextKey()), &emptypb.Empty{})
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) Uninstall(ctx context.Context, in ContextKeyable) (*emptypb.Empty, error) {
	return s.base.Uninstall(contextWithKey(ctx, in.ContextKey()), &emptypb.Empty{})
}

func (s *ContextKeyableConfigServer[G, S, R, H, HR, T]) Tracker() *DefaultingConfigTracker[T] {
	return s.base.tracker
}
