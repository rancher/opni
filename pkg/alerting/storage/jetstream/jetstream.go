package jetstream

import (
	"bytes"
	"context"
	"errors"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	storage_opts "github.com/rancher/opni/pkg/alerting/storage/opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v2"
)

type JetstreamRouterStore[T routing.OpniRouting] struct {
	obj      nats.ObjectStore
	basePath string
}

type JetStreamAlertingStorage[T interfaces.AlertingSecret] struct {
	kv       nats.KeyValue
	basePath string
}

type JetStreamAlertingStateCache struct {
	// storage.AlertingStorage[*alertingv1.CachedState]
	*JetStreamAlertingStorage[*alertingv1.CachedState]
	stateMu sync.RWMutex
}

type JetStreamAlertingIncidentTracker struct {
	// storage.AlertingStorage[*alertingv1.IncidentIntervals]
	*JetStreamAlertingStorage[*alertingv1.IncidentIntervals]
	ttl time.Duration
}

// Responsible for anything related to persistent storage
// external to AlertManager

func NewJetStreamAlertingStorage[T interfaces.AlertingSecret](
	kv nats.KeyValue,
	basePath string,
) *JetStreamAlertingStorage[T] {
	return &JetStreamAlertingStorage[T]{
		kv:       kv,
		basePath: basePath,
	}
}

func NewJetStreamAlertingStateCache(kv nats.KeyValue, prefix string) *JetStreamAlertingStateCache {
	return &JetStreamAlertingStateCache{
		JetStreamAlertingStorage: NewJetStreamAlertingStorage[*alertingv1.CachedState](kv, prefix),
	}
}

func NewJetStreamAlertingIncidentTracker(kv nats.KeyValue, prefix string, ttl time.Duration) *JetStreamAlertingIncidentTracker {
	return &JetStreamAlertingIncidentTracker{
		JetStreamAlertingStorage: NewJetStreamAlertingStorage[*alertingv1.IncidentIntervals](kv, prefix),
		ttl:                      ttl,
	}
}

func NewJetstreamRouterStore(obj nats.ObjectStore, prefix string) *JetstreamRouterStore[routing.OpniRouting] {
	return &JetstreamRouterStore[routing.OpniRouting]{
		obj:      obj,
		basePath: prefix,
	}
}

func (j JetstreamRouterStore[T]) key(k string) string {
	return path.Join(j.basePath, k)
}

func (j JetstreamRouterStore[T]) Get(_ context.Context, key string, opts ...storage_opts.RequestOption) (T, error) {
	options := storage_opts.RequestOptions{}
	options.Apply(opts...)
	var t T
	objRes, err := j.obj.Get(j.key(key))
	if err != nil {
		return t, err
	}
	// var data interface{}
	tType := reflect.TypeOf(t)
	rt := reflect.New(tType.Elem()).Interface().(T)

	err = yaml.NewDecoder(objRes).Decode(&rt)
	if err != nil {
		return t, err
	}
	return rt, nil
}

func (j JetstreamRouterStore[T]) Put(_ context.Context, key string, value T) error {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	_, err = j.obj.Put(&nats.ObjectMeta{
		Name: j.key(key),
	}, bytes.NewReader(raw))
	return err
}

func (j JetstreamRouterStore[T]) ListKeys(_ context.Context) ([]string, error) {
	var keys []string
	objs, err := j.obj.List()
	if errors.Is(err, nats.ErrNoKeysFound) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		keys = append(keys, obj.Name)
	}
	return keys, nil
}

func (j JetstreamRouterStore[T]) List(ctx context.Context, opts ...storage_opts.RequestOption) ([]T, error) {
	var routers []T
	keys, err := j.ListKeys(ctx)
	if err != nil {
		return routers, err
	}
	for _, key := range keys {
		router, err := j.Get(ctx, key, opts...)
		if err != nil {
			return routers, err
		}
		routers = append(routers, router)
	}
	return routers, nil
}

func (j JetstreamRouterStore[T]) Delete(_ context.Context, key string) error {
	return j.obj.Delete(j.key(key))
}

func (j *JetStreamAlertingStorage[T]) Key(key string) string {
	return path.Join(j.basePath, key)
}

func (j *JetStreamAlertingStorage[T]) Put(_ context.Context, key string, value T) error {
	opts := protojson.MarshalOptions{
		AllowPartial: true,
	}
	data, err := opts.Marshal(value)
	if err != nil {
		return err
	}
	_, err = j.kv.Put(j.Key(key), data)
	return err
}

func (j *JetStreamAlertingStorage[T]) Get(_ context.Context, key string, opts ...storage_opts.RequestOption) (T, error) {
	var t T
	options := storage_opts.RequestOptions{}
	options.Apply(opts...)
	data, err := j.kv.Get(j.Key(key))
	if err != nil {
		return t, err
	}
	// version migrations/ missing fields should be patched when manipulated by the alerting plugin
	unmarshalOpts := protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
	tType := reflect.TypeOf(t)
	rt := reflect.New(tType.Elem()).Interface().(T)
	err = unmarshalOpts.Unmarshal(data.Value(), rt)
	if err != nil {
		return t, err
	}
	if !options.Unredacted {
		rt.RedactSecrets()
	}
	return rt, nil
}

func (j *JetStreamAlertingStorage[T]) Delete(_ context.Context, key string) error {
	err := j.kv.Delete(j.Key(key))
	if errors.Is(err, nats.ErrKeyNotFound) {
		return nil
	}
	return err
}

func (j *JetStreamAlertingStorage[T]) ListKeys(_ context.Context) ([]string, error) {
	keys, err := j.kv.Keys()
	if errors.Is(err, nats.ErrNoKeysFound) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}
	k := lo.Filter(keys, func(path string, _ int) bool {
		return strings.HasPrefix(path, j.basePath)
	})

	return lo.Map(k, func(value string, _ int) string {
		return path.Base(value)
	}), nil
}

func (j *JetStreamAlertingStorage[T]) List(ctx context.Context, opts ...storage_opts.RequestOption) ([]T, error) {
	keys, err := j.ListKeys(ctx)
	if err != nil {
		return nil, err
	}

	var values []T
	for _, key := range keys {
		value, err := j.Get(ctx, key, opts...)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (j *JetStreamAlertingStateCache) IsDiff(ctx context.Context, key string, incomingState *alertingv1.CachedState) bool {
	persistedState, err := j.JetStreamAlertingStorage.Get(ctx, key, storage_opts.WithUnredacted())
	if err != nil {
		// if it's not found, then the state is definitely different, otherwise we assume the resource is busy
		return errors.Is(err, nats.ErrKeyNotFound)
	}
	return !persistedState.IsEquivalent(incomingState)
}

func (j *JetStreamAlertingStateCache) LastKnownChange(ctx context.Context, key string) (*timestamppb.Timestamp, error) {
	state, err := j.JetStreamAlertingStorage.Get(ctx, key, storage_opts.WithUnredacted())
	if err != nil {
		return nil, err
	}
	return state.GetTimestamp(), nil
}

func (j *JetStreamAlertingStateCache) Put(ctx context.Context, key string, incomingState *alertingv1.CachedState) error {
	j.stateMu.Lock()
	defer j.stateMu.Unlock()
	if j.IsDiff(ctx, key, incomingState) {
		return j.JetStreamAlertingStorage.Put(ctx, key, incomingState)
	}
	// check timestamps
	lastKnownChange, err := j.LastKnownChange(ctx, key)
	if err != nil {
		return nil
	}
	if lastKnownChange.AsTime().After(incomingState.GetTimestamp().AsTime()) {
		return j.JetStreamAlertingStorage.Put(ctx, key, incomingState)
	}
	return nil
}

func (j *JetStreamAlertingStateCache) Get(ctx context.Context, key string, opts ...storage_opts.RequestOption) (*alertingv1.CachedState, error) {
	j.stateMu.RLock()
	defer j.stateMu.RUnlock()
	return j.JetStreamAlertingStorage.Get(ctx, key, opts...)
}

func (j *JetStreamAlertingIncidentTracker) OpenInterval(ctx context.Context, conditionId string, start *timestamppb.Timestamp) error {
	// get
	existingIntervals, err := j.JetStreamAlertingStorage.Get(ctx, conditionId)
	var intervals *alertingv1.IncidentIntervals
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			// Create a new entry
			intervals = &alertingv1.IncidentIntervals{
				Items: []*alertingv1.Interval{},
			}
			err = j.JetStreamAlertingStorage.Put(ctx, conditionId, intervals)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		intervals = existingIntervals
	}

	// update
	if len(intervals.Items) == 0 {
		intervals.Items = append(intervals.Items, &alertingv1.Interval{
			Start: start,
			End:   nil,
		})
	} else {
		last := intervals.Items[len(intervals.Items)-1]
		if last.Start == nil {
			last.Start = start
		} else {
			last.End = start
			intervals.Items = append(intervals.Items, &alertingv1.Interval{
				Start: start,
				End:   nil,
			})
		}
	}
	intervals.Prune(j.ttl)
	return j.JetStreamAlertingStorage.Put(ctx, conditionId, intervals)
}

func (j *JetStreamAlertingIncidentTracker) CloseInterval(ctx context.Context, conditionId string, end *timestamppb.Timestamp) error {
	// get
	existingIntervals, err := j.JetStreamAlertingStorage.Get(ctx, conditionId)
	var intervals *alertingv1.IncidentIntervals
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			// Create a new entry
			intervals = &alertingv1.IncidentIntervals{
				Items: []*alertingv1.Interval{},
			}
			err = j.JetStreamAlertingStorage.Put(ctx, conditionId, intervals)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		intervals = existingIntervals
	}

	// put
	if len(intervals.Items) == 0 { // weird
		intervals.Items = append(intervals.Items, &alertingv1.Interval{
			Start: end,
			End:   end,
		})
	} else {
		last := intervals.Items[len(intervals.Items)-1]
		if last.Start == nil { //weird
			last.Start = end
			last.End = end
		} else {
			last.End = end
		}
	}
	intervals.Prune(j.ttl)
	return j.JetStreamAlertingStorage.Put(ctx, conditionId, intervals)
}

func (j *JetStreamAlertingIncidentTracker) GetActiveWindowsFromIncidentTracker(
	ctx context.Context,
	conditionId string,
	start,
	end *timestamppb.Timestamp,
) ([]*alertingv1.ActiveWindow, error) {
	incidents, err := j.JetStreamAlertingStorage.Get(ctx, conditionId)
	if err != nil {
		return nil, err
	}
	res := []*alertingv1.ActiveWindow{}
	if len(incidents.Items) == 0 {
		return res, nil
	}
	for _, step := range incidents.Items {
		if step.Start.AsTime().Before(start.AsTime()) {
			continue
		}
		if step.Start == step.End {
			continue
		}
		window := &alertingv1.ActiveWindow{
			Start: step.Start,
			End:   step.End, // overwritten if it is found later
			Type:  alertingv1.TimelineType_Timeline_Alerting,
		}
		if window.End == nil {
			window.End = end
		}
		res = append(res, window)
	}
	return res, nil
}
