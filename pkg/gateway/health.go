package gateway

import (
	"context"
	"errors"
	"path"
	"strings"
	sync "sync"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HealthStatusWriterManager struct {
	healthQueue        chan health.HealthUpdate
	statusQueue        chan health.StatusUpdate
	queueLock          sync.Mutex
	healthQueueByAgent map[string]chan health.HealthUpdate
	statusQueueByAgent map[string]chan health.StatusUpdate
	connectionsKv      storage.KeyValueStore
	logger             *zap.SugaredLogger
}

var _ TrackedConnectionListener = (*HealthStatusWriterManager)(nil)

func NewHealthStatusWriterManager(ctx context.Context, rootKv storage.KeyValueStore, lg *zap.SugaredLogger) *HealthStatusWriterManager {
	healthQueue := make(chan health.HealthUpdate, 256)
	statusQueue := make(chan health.StatusUpdate, 256)
	mgr := &HealthStatusWriterManager{
		connectionsKv:      rootKv,
		logger:             lg,
		healthQueue:        healthQueue,
		statusQueue:        statusQueue,
		healthQueueByAgent: make(map[string]chan health.HealthUpdate),
		statusQueueByAgent: make(map[string]chan health.StatusUpdate),
	}
	lg.Debug("starting health status writer manager")
	go func() {
		defer lg.Debug("stopping health status writer manager")
		for {
			select {
			case <-ctx.Done():
				return
			case h := <-healthQueue:
				mgr.agentHealthQueue(h.ID) <- h
			case s := <-statusQueue:
				mgr.agentStatusQueue(s.ID) <- s
			}
		}
	}()
	return mgr
}

func (w *HealthStatusWriterManager) agentHealthQueue(id string) chan health.HealthUpdate {
	w.queueLock.Lock()
	defer w.queueLock.Unlock()
	if q, ok := w.healthQueueByAgent[id]; ok {
		return q
	}
	q := make(chan health.HealthUpdate, 8)
	w.logger.Debugf("initializing health queue for %s", id)
	w.healthQueueByAgent[id] = q
	return q
}

func (w *HealthStatusWriterManager) agentStatusQueue(id string) chan health.StatusUpdate {
	w.queueLock.Lock()
	defer w.queueLock.Unlock()
	if q, ok := w.statusQueueByAgent[id]; ok {
		return q
	}
	q := make(chan health.StatusUpdate, 8)
	w.logger.Debugf("initializing health queue for %s", id)
	w.statusQueueByAgent[id] = q
	return q
}

// HealthWriterC implements health.HealthStatusUpdateWriter.
func (w *HealthStatusWriterManager) HealthWriterC() chan<- health.HealthUpdate {
	return w.healthQueue
}

// StatusWriterC implements health.HealthStatusUpdateWriter.
func (w *HealthStatusWriterManager) StatusWriterC() chan<- health.StatusUpdate {
	return w.statusQueue
}

// Close implements health.HealthStatusUpdateWriter.
func (w *HealthStatusWriterManager) Close() {
	close(w.healthQueue)
	close(w.statusQueue)
}

// HandleTrackedConnection implements RemoteConnectionListener.
func (wm *HealthStatusWriterManager) HandleTrackedConnection(ctx context.Context, agentId string, leaseId string, instanceInfo *corev1.InstanceInfo) {
	w := NewHealthStatusWriter(ctx, kvutil.WithMessageCodec[*corev1.InstanceInfo](kvutil.WithKey(wm.connectionsKv, path.Join(agentId, leaseId))), wm.logger)
	agentBuffer := health.AsBuffer(wm.agentHealthQueue(agentId), wm.agentStatusQueue(agentId))
	wm.logger.Debugf("handling new tracked connection for %s", agentId)
	go func() {
		defer w.Close()
		health.Copy(ctx, w, agentBuffer)
	}()
}

type HealthStatusWriter struct {
	v       storage.ValueStoreT[*corev1.InstanceInfo]
	healthC chan health.HealthUpdate
	statusC chan health.StatusUpdate
}

var _ health.HealthStatusUpdateWriter = (*HealthStatusWriter)(nil)

// Writes health and status updates to the keys "health" and "status" at the
// root of the provided KeyValueStore. To write updates to a specific path,
// use a prefixed KeyValueStore.
func NewHealthStatusWriter(ctx context.Context, v storage.ValueStoreT[*corev1.InstanceInfo], lg *zap.SugaredLogger) health.HealthStatusUpdateWriter {
	healthC := make(chan health.HealthUpdate, 8)
	statusC := make(chan health.StatusUpdate, 8)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case h := <-healthC:
				var rev int64
				ii, err := v.Get(ctx, storage.WithRevisionOut(&rev))
				if err != nil && !storage.IsNotFound(err) {
					if errors.Is(err, ctx.Err()) {
						return
					}
					lg.With(zap.Error(err)).Error("failed to get instance info")
					continue
				}
				if ii == nil {
					ii = &corev1.InstanceInfo{}
				}
				ii.Health = h.Health
				if err := v.Put(ctx, ii, storage.WithRevision(rev)); err != nil {
					lg.With(zap.Error(err)).Error("failed to put instance info")
					continue
				}
			case s := <-statusC:
				var rev int64
				ii, err := v.Get(ctx, storage.WithRevisionOut(&rev))
				if err != nil && !storage.IsNotFound(err) {
					if errors.Is(err, ctx.Err()) {
						return
					}
					lg.With(zap.Error(err)).Error("failed to get instance info")
					continue
				}
				if ii == nil {
					ii = &corev1.InstanceInfo{}
				}
				ii.Status = s.Status
				if err := v.Put(ctx, ii, storage.WithRevision(rev)); err != nil {
					lg.With(zap.Error(err)).Error("failed to put instance info")
					continue
				}
			}
		}
	}()

	return &HealthStatusWriter{
		v:       v,
		healthC: healthC,
		statusC: statusC,
	}
}

// HealthWriterC implements health.HealthStatusUpdateWriter.
func (w *HealthStatusWriter) HealthWriterC() chan<- health.HealthUpdate {
	return w.healthC
}

// StatusWriterC implements health.HealthStatusUpdateWriter.
func (w *HealthStatusWriter) StatusWriterC() chan<- health.StatusUpdate {
	return w.statusC
}

func (w *HealthStatusWriter) Close() {
	close(w.healthC)
	close(w.statusC)
}

type HealthStatusReader struct {
	kv      storage.KeyValueStore
	healthC chan health.HealthUpdate
	statusC chan health.StatusUpdate
}

var _ health.HealthStatusUpdateReader = (*HealthStatusReader)(nil)

type HealthStatusReaderOptions struct {
	filter func(id string) bool
}

type HealthStatusReaderOption func(*HealthStatusReaderOptions)

func (o *HealthStatusReaderOptions) apply(opts ...HealthStatusReaderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithFilter(filter func(id string) bool) HealthStatusReaderOption {
	return func(o *HealthStatusReaderOptions) {
		o.filter = filter
	}
}
func NewHealthStatusReader(ctx context.Context, kv storage.KeyValueStore, opts ...HealthStatusReaderOption) (health.HealthStatusUpdateReader, error) {
	options := HealthStatusReaderOptions{
		filter: func(_ string) bool { return true },
	}
	options.apply(opts...)

	healthC := make(chan health.HealthUpdate, 256)
	statusC := make(chan health.StatusUpdate, 256)
	events, err := kv.Watch(ctx, "", storage.WithPrefix(), storage.WithRevision(0))
	if err != nil {
		return nil, err
	}

	decodeKey := func(key string) (string, string, bool) {
		parts := strings.Split(key, "/")
		if len(parts) != 2 {
			return "", "", false
		}
		return parts[0], parts[1], true
	}

	healthTime := time.Time{}
	statusTime := time.Time{}

	go func() {
		defer close(healthC)
		defer close(statusC)

		for event := range events {
			switch event.EventType {
			case storage.WatchEventPut:
				id, _, ok := decodeKey(event.Current.Key())
				if !ok {
					// fmt.Println("could not decode key")
					continue
				}
				if !options.filter(id) {
					// fmt.Println("excluded by filter")
					continue
				}

				var info corev1.InstanceInfo
				if err := proto.Unmarshal(event.Current.Value(), &info); err != nil {
					// fmt.Println("could not unmarshal instance info")
					continue
				}
				if !info.GetAcquired() {
					// a different instance is only attempting to acquire the lock,
					// ignore the event
					continue
				}
				updatedHealthTime := info.GetHealth().GetTimestamp().AsTime()
				if info.Health != nil {
					if updatedHealthTime.After(healthTime) {
						healthC <- health.HealthUpdate{
							ID:     id,
							Health: info.Health,
						}
					}
				} else {
					healthC <- health.HealthUpdate{
						ID: id,
						Health: &corev1.Health{
							Timestamp: timestamppb.Now(),
							Ready:     false,
						},
					}
				}
				healthTime = updatedHealthTime

				updatedStatusTime := info.GetStatus().GetTimestamp().AsTime()
				if info.Status != nil {
					if updatedStatusTime.After(statusTime) {
						statusC <- health.StatusUpdate{
							ID:     id,
							Status: info.Status,
						}
					}
				} else {
					statusC <- health.StatusUpdate{
						ID: id,
						Status: &corev1.Status{
							Timestamp: timestamppb.Now(),
							Connected: false,
						},
					}
				}
				statusTime = updatedStatusTime
			case storage.WatchEventDelete:
				id, _, ok := decodeKey(event.Previous.Key())
				if !ok || !options.filter(id) {
					continue
				}
				var info corev1.InstanceInfo
				if err := proto.Unmarshal(event.Previous.Value(), &info); err != nil {
					// fmt.Println("could not unmarshal instance info")
					continue
				}
				if !info.GetAcquired() {
					// a different instance likely tried to lock the key and failed,
					// deleting its mutex key, so ignore the event
					continue
				}

				healthC <- health.HealthUpdate{
					ID: id,
					Health: &corev1.Health{
						Timestamp: timestamppb.Now(),
						Ready:     false,
					},
				}
				healthTime = time.Time{}
				statusC <- health.StatusUpdate{
					ID: id,
					Status: &corev1.Status{
						Timestamp: timestamppb.Now(),
						Connected: false,
					},
				}
				statusTime = time.Time{}
			}
		}
	}()
	return &HealthStatusReader{
		kv:      kv,
		healthC: healthC,
		statusC: statusC,
	}, nil
}

// HealthC implements health.HealthStatusUpdater.
func (r *HealthStatusReader) HealthC() <-chan health.HealthUpdate {
	return r.healthC
}

// StatusC implements health.HealthStatusUpdater.
func (r *HealthStatusReader) StatusC() <-chan health.StatusUpdate {
	return r.statusC
}
