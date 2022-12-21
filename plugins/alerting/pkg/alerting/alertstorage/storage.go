package alertstorage

import (
	"context"
	"errors"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const conditionPrefix = "/alerting/conditions"
const endpointPrefix = "/alerting/endpoints"
const statePrefix = "/alerting/state"
const incidentPrefix = "/alerting/incidents"
const defaultTrackerTTL = 24 * time.Hour

type AlertingStorage[T interfaces.AlertingSecret] interface {
	Put(ctx context.Context, key string, value T) error
	Get(ctx context.Context, key string, opts ...RequestOption) (T, error)
	Delete(ctx context.Context, key string) error
	ListKeys(ctx context.Context) ([]string, error)
	List(ctx context.Context, opts ...RequestOption) ([]T, error)
}

type AlertingStateCache[T interfaces.AlertingSecret] interface {
	AlertingStorage[T]
	IsDiff(ctx context.Context, key string, value T) bool
	LastKnownChange(ctx context.Context, key string) (*timestamppb.Timestamp, error)
}

type AlertingIncidentTracker[T interfaces.AlertingSecret] interface {
	AlertingStorage[T]
	OpenInterval(ctx context.Context, conditionId string, start *timestamppb.Timestamp) error
	CloseInterval(ctx context.Context, conditionId string, end *timestamppb.Timestamp) error
	GetActiveWindowsFromIncidentTracker(
		ctx context.Context,
		conditionId string,
		start,
		end *timestamppb.Timestamp,
	) ([]*alertingv1.ActiveWindow, error)
}

type RequestOptions struct {
	Unredacted bool
}

type RequestOption func(*RequestOptions)

func WithUnredacted() RequestOption {
	return func(o *RequestOptions) {
		o.Unredacted = true
	}
}

func (o *RequestOptions) apply(opts ...RequestOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type JetStreamAlertingStorage[T interfaces.AlertingSecret] struct {
	kv       nats.KeyValue
	basePath string
}

var _ AlertingStorage[interfaces.AlertingSecret] = (*JetStreamAlertingStorage[interfaces.AlertingSecret])(nil)

type JetStreamAlertingStateCache struct {
	JetStreamAlertingStorage[*alertingv1.CachedState]
	stateMu sync.RWMutex
}

var _ AlertingStateCache[*alertingv1.CachedState] = (*JetStreamAlertingStateCache)(nil)

type JetStreamAlertingIncidentTracker struct {
	JetStreamAlertingStorage[*alertingv1.IncidentIntervals]
	ttl time.Duration
}

var _ AlertingIncidentTracker[*alertingv1.IncidentIntervals] = (*JetStreamAlertingIncidentTracker)(nil)

// Responsible for anything related to persistent storage
// external to AlertManager
type StorageNode struct {
	StorageAPIs
	StorageNodeOptions
}
type StorageNodeOptions struct {
	Logger     *zap.SugaredLogger
	timeout    time.Duration
	trackerTTl time.Duration
}

func NewStorageNode(s StorageAPIs, opts ...StorageNodeOption) *StorageNode {
	options := StorageNodeOptions{
		timeout:    5 * time.Second,
		trackerTTl: defaultTrackerTTL,
	}
	if options.Logger != nil {
		options.Logger = logger.NewPluginLogger().Named("alerting-storage-node")

	}
	options.apply(opts...)
	return &StorageNode{
		StorageAPIs: s,
	}
}

type StorageNodeOption func(*StorageNodeOptions)

func WithStorageTimeout(timeout time.Duration) StorageNodeOption {
	return func(o *StorageNodeOptions) {
		o.timeout = timeout
	}
}

func WithLogger(lg *zap.SugaredLogger) StorageNodeOption {
	return func(o *StorageNodeOptions) {
		o.Logger = lg
	}
}

func (s *StorageNodeOptions) apply(opts ...StorageNodeOption) {
	for _, opt := range opts {
		opt(s)
	}
}

func NewJetStreamAlertingStorage[T interfaces.AlertingSecret](
	kv nats.KeyValue,
	basePath string,
) JetStreamAlertingStorage[T] {
	return JetStreamAlertingStorage[T]{
		kv:       kv,
		basePath: basePath,
	}
}

func NewJetStreamAlertingStateCache(kv nats.KeyValue, prefix string) *JetStreamAlertingStateCache {
	return &JetStreamAlertingStateCache{
		JetStreamAlertingStorage: NewJetStreamAlertingStorage[*alertingv1.CachedState](kv, statePrefix),
	}
}

func NewJetStreamAlertingIncidentTracker(kv nats.KeyValue, prefix string, ttl time.Duration) *JetStreamAlertingIncidentTracker {
	return &JetStreamAlertingIncidentTracker{
		JetStreamAlertingStorage: NewJetStreamAlertingStorage[*alertingv1.IncidentIntervals](kv, incidentPrefix),
		ttl:                      ttl,
	}
}

var _ AlertingStorage[interfaces.AlertingSecret] = (*JetStreamAlertingStorage[interfaces.AlertingSecret])(nil)

func (j *JetStreamAlertingStorage[T]) Key(key string) string {
	return path.Join(j.basePath, key)
}

func (j *JetStreamAlertingStorage[T]) Put(ctx context.Context, key string, value T) error {
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

func (j *JetStreamAlertingStorage[T]) Get(ctx context.Context, key string, opts ...RequestOption) (T, error) {
	var t T
	options := RequestOptions{}
	options.apply(opts...)
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

func (j *JetStreamAlertingStorage[T]) Delete(ctx context.Context, key string) error {
	err := j.kv.Delete(j.Key(key))
	if errors.Is(err, nats.ErrKeyNotFound) {
		return nil
	}
	return err
}

func (j *JetStreamAlertingStorage[T]) ListKeys(ctx context.Context) ([]string, error) {
	keys, err := j.kv.Keys()
	if err != nil {
		return nil, err
	}
	k := lo.Filter(keys, func(path string, _ int) bool {
		return strings.HasPrefix(path, j.basePath)
	})

	return lo.Map(k, func(value string, _ int) string {
		return path.Base(value)
	}), nil
}

func (j *JetStreamAlertingStorage[T]) List(ctx context.Context, opts ...RequestOption) ([]T, error) {
	keys, err := j.ListKeys(ctx)
	if err != nil {
		return nil, err
	}
	if errors.Is(err, nats.ErrNoKeysFound) {
		return []T{}, nil
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

type StorageAPIs struct {
	Conditions JetStreamAlertingStorage[*alertingv1.AlertCondition]
	Endpoints  JetStreamAlertingStorage[*alertingv1.AlertEndpoint]

	StateCache    *JetStreamAlertingStateCache
	IncidentCache *JetStreamAlertingIncidentTracker
}

func NewStorageAPIs(js nats.JetStreamContext, ttl time.Duration) StorageAPIs {
	return StorageAPIs{
		Conditions: NewJetStreamAlertingStorage[*alertingv1.AlertCondition](
			NewConditionKeyStore(js),
			conditionPrefix,
		),
		Endpoints: NewJetStreamAlertingStorage[*alertingv1.AlertEndpoint](
			NewEndpointKeyStore(js),
			endpointPrefix,
		),
		StateCache: NewJetStreamAlertingStateCache(
			NewStatusCache(js),
			statePrefix,
		),
		IncidentCache: NewJetStreamAlertingIncidentTracker(
			NewIncidentKeyStore(js),
			incidentPrefix,
			ttl,
		),
	}
}

func (j *JetStreamAlertingStateCache) IsDiff(ctx context.Context, key string, incomingState *alertingv1.CachedState) bool {
	persistedState, err := j.JetStreamAlertingStorage.Get(ctx, key, WithUnredacted())
	if err != nil {
		// if it's not found, then the state is definitely different, otherwise we assume the resource is busy
		return errors.Is(err, nats.ErrKeyNotFound)
	}
	return !persistedState.IsEquivalent(incomingState)
}

func (j *JetStreamAlertingStateCache) LastKnownChange(ctx context.Context, key string) (*timestamppb.Timestamp, error) {
	state, err := j.JetStreamAlertingStorage.Get(ctx, key, WithUnredacted())
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
	} else {
		// check timestamps
		lastKnownChange, err := j.LastKnownChange(ctx, key)
		if err != nil {
			return nil
		}
		if lastKnownChange.AsTime().After(incomingState.GetTimestamp().AsTime()) {
			return j.JetStreamAlertingStorage.Put(ctx, key, incomingState)
		}
	}
	return nil
}

func (j *JetStreamAlertingStateCache) Get(ctx context.Context, key string, opts ...RequestOption) (*alertingv1.CachedState, error) {
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
