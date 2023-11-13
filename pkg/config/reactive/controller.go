package reactive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/nsf/jsondiff"
	art "github.com/plar/go-adaptive-radix-tree"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/fieldmask"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type DiffMode int

const (
	DiffStat DiffMode = iota
	DiffFull
)

type ControllerOptions struct {
	logger   *slog.Logger
	diffMode DiffMode
}

type ControllerOption func(*ControllerOptions)

func (o *ControllerOptions) apply(opts ...ControllerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLogger(eventLogger *slog.Logger) ControllerOption {
	return func(o *ControllerOptions) {
		o.logger = eventLogger
	}
}

func WithDiffMode(mode DiffMode) ControllerOption {
	return func(o *ControllerOptions) {
		o.diffMode = mode
	}
}

type Controller[T driverutil.ConfigType[T]] struct {
	ControllerOptions
	tracker *driverutil.DefaultingConfigTracker[T]

	runOnce    atomic.Bool
	runContext context.Context

	reactiveMessagesMu sync.Mutex
	reactiveMessages   art.Tree

	currentRevMu sync.Mutex
	currentRev   int64
}

func NewController[T driverutil.ConfigType[T]](tracker *driverutil.DefaultingConfigTracker[T], opts ...ControllerOption) *Controller[T] {
	options := ControllerOptions{}
	options.apply(opts...)

	return &Controller[T]{
		ControllerOptions: options,
		tracker:           tracker,
		reactiveMessages:  art.New(),
	}
}

func (s *Controller[T]) Start(ctx context.Context) error {
	if !s.runOnce.CompareAndSwap(false, true) {
		panic("bug: Run called twice")
	}
	s.runContext = ctx

	var rev int64
	_, err := s.tracker.ActiveStore().Get(ctx, storage.WithRevisionOut(&rev))
	if err != nil {
		if !storage.IsNotFound(err) {
			return err
		}
	}
	w, err := s.tracker.ActiveStore().Watch(ctx, storage.WithRevision(rev))
	if err != nil {
		return err
	}
	if rev != 0 {
		// The first event must be handled before this function returns, if
		// there is an existing configuration. Otherwise, a logic race will occur
		// between the goroutine below and calls to Reactive() after this
		// function returns. New reactive values have late-join initialization
		// logic; if the first event is not consumed, the late-join logic and
		// the goroutine below (when it is scheduled) would cause duplicate
		// updates to be sent to newly created reactive values.
		firstEvent, ok := <-w
		if !ok {
			return fmt.Errorf("watch channel closed unexpectedly")
		}

		// At this point there will most likely not be any reactive values, but
		// this function sets s.currentRev and also logs the first event.
		s.handleWatchEvent(firstEvent)
	}
	go func() {
		for {
			cfg, ok := <-w
			if !ok {
				return
			}
			s.handleWatchEvent(cfg)
		}
	}()
	return nil
}

func (s *Controller[T]) handleWatchEvent(cfg storage.WatchEvent[storage.KeyRevision[T]]) {
	s.reactiveMessagesMu.Lock()
	defer s.reactiveMessagesMu.Unlock()

	group := make(chan struct{})
	defer func() {
		close(group)
	}()

	switch cfg.EventType {
	case storage.WatchEventDelete:
		if s.logger != nil {
			s.logger.With(
				"key", cfg.Previous.Key(),
				"prevRevision", cfg.Previous.Revision(),
			).Info("configuration deleted")
		}
		s.reactiveMessages.ForEach(func(node art.Node) (cont bool) {
			rm := node.Value().(*reactiveValue)
			rm.Update(cfg.Previous.Revision(), protoreflect.Value{}, group)
			return true
		})
	case storage.WatchEventPut:
		s.currentRevMu.Lock()
		s.currentRev = cfg.Current.Revision()
		s.currentRevMu.Unlock()

		// efficiently compute a list of paths (or prefixes) that have changed
		var prevValue T
		if cfg.Previous != nil {
			prevValue = cfg.Previous.Value()
		}
		diffMask := fieldmask.Diff(prevValue, cfg.Current.Value())

		if s.logger != nil {
			opts := jsondiff.DefaultConsoleOptions()
			opts.SkipMatches = true
			diff, _ := driverutil.RenderJsonDiff(prevValue, cfg.Current.Value(), opts)
			stat := driverutil.DiffStat(diff, opts)
			switch s.diffMode {
			case DiffStat:
				s.logger.Info("configuration updated", "revision", cfg.Current.Revision(), "diff", stat)
			case DiffFull:
				s.logger.Info("configuration updated", "revision", cfg.Current.Revision(), "diff", stat)
				s.logger.Info("⤷ diff:\n" + diff)
			}
		}

		// parent message watchers are updated when any of their fields change,
		// but only once
		implicitParentUpdates := make(map[string]protoreflect.Value)
		for _, path := range diffMask.Paths {
			// search reactive messages by prefix path
			s.reactiveMessages.ForEachPrefix(art.Key(path), func(node art.Node) bool {
				if node.Kind() != art.Leaf {
					return true
				}
				key := string(node.Key())
				parts := strings.Split(key, ".")

				// get the new value of the current message
				value := protoreflect.ValueOf(cfg.Current.Value().ProtoReflect())
				for i, part := range parts {
					value = value.Message().Get(value.Message().Descriptor().Fields().ByName(protoreflect.Name(part)))
					if i < len(parts)-1 {
						key := strings.Join(parts[:i+1], ".")
						if _, exists := implicitParentUpdates[key]; !exists {
							implicitParentUpdates[key] = value
						}
					}
				}
				// update the reactive messages
				rm := node.Value().(*reactiveValue)
				rm.Update(cfg.Current.Revision(), value, group)
				return true
			})
		}
		for key, value := range implicitParentUpdates {
			v, exists := s.reactiveMessages.Search(art.Key(key))
			if exists {
				// update the parent reactive message
				rm := v.(*reactiveValue)
				rm.Update(cfg.Current.Revision(), value, group)
			}
		}
	}
}

func (s *Controller[T]) Reactive(path protopath.Path) Value {
	if len(path) < 2 || path[0].Kind() != protopath.RootStep {
		panic(fmt.Sprintf("invalid reactive message path: %s", path))
	}
	s.currentRevMu.Lock()
	currentConfig, _ := s.tracker.ActiveStore().Get(s.runContext, storage.WithRevision(s.currentRev))
	s.currentRevMu.Unlock()

	s.reactiveMessagesMu.Lock()
	defer s.reactiveMessagesMu.Unlock()

	var currentValue protoreflect.Value
	if currentConfig.ProtoReflect().IsValid() {
		currentValue = protoreflect.ValueOfMessage(currentConfig.ProtoReflect())
	}
	for _, step := range path {
		switch step.Kind() {
		case protopath.RootStep:
			continue
		case protopath.FieldAccessStep:
			if currentConfig.ProtoReflect().IsValid() {
				currentValue = currentValue.Message().Get(step.FieldDescriptor())
			}
		default:
			panic("bug: invalid reactive message path: " + path.String())
		}
	}

	key := path[1:].String()[1:]
	v, exists := s.reactiveMessages.Search(art.Key(key))
	if exists {
		return v.(*reactiveValue)
	}
	rm := &reactiveValue{}
	rm.Update(s.currentRev, currentValue, nil)
	s.reactiveMessages.Insert(art.Key(key), rm)
	return rm
}
