package gateway

import (
	"context"
	"strings"
	sync "sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
)

type HealthStatusWriterManager struct {
	healthQueue        chan health.HealthUpdate
	statusQueue        chan health.StatusUpdate
	queueLock          sync.Mutex
	healthQueueByAgent map[string]chan health.HealthUpdate
	statusQueueByAgent map[string]chan health.StatusUpdate
	rootKv             storage.KeyValueStore
	logger             *zap.SugaredLogger
}

var _ TrackedConnectionListener = (*HealthStatusWriterManager)(nil)

func NewHealthStatusWriterManager(ctx context.Context, rootKv storage.KeyValueStore, lg *zap.SugaredLogger) *HealthStatusWriterManager {
	healthQueue := make(chan health.HealthUpdate, 256)
	statusQueue := make(chan health.StatusUpdate, 256)
	mgr := &HealthStatusWriterManager{
		rootKv:             rootKv,
		logger:             lg,
		healthQueue:        healthQueue,
		statusQueue:        statusQueue,
		healthQueueByAgent: make(map[string]chan health.HealthUpdate),
		statusQueueByAgent: make(map[string]chan health.StatusUpdate),
	}
	go func() {
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

// HandleTrackedConnection implements RemoteConnectionListener.
func (wm *HealthStatusWriterManager) HandleTrackedConnection(ctx context.Context, agentId string, prefix string, instanceInfo *corev1.InstanceInfo) {
	wm.queueLock.Lock()
	defer wm.queueLock.Unlock()
	w := NewHealthStatusWriter(ctx, kvutil.WithPrefix(wm.rootKv, prefix+"/"), wm.logger)
	var agentHealthQueue chan health.HealthUpdate
	var agentStatusQueue chan health.StatusUpdate
	if q, ok := wm.healthQueueByAgent[agentId]; !ok {
		agentHealthQueue = make(chan health.HealthUpdate, 8)
		wm.healthQueueByAgent[agentId] = agentHealthQueue
	} else {
		agentHealthQueue = q
	}
	if q, ok := wm.statusQueueByAgent[agentId]; !ok {
		agentStatusQueue = make(chan health.StatusUpdate, 8)
		wm.statusQueueByAgent[agentId] = agentStatusQueue
	} else {
		agentStatusQueue = q
	}
	go func() {
		healthWriter := w.HealthWriterC()
		statusWriter := w.StatusWriterC()
		defer close(healthWriter)
		defer close(statusWriter)
		for {
			select {
			case <-ctx.Done():
				return
			case h := <-agentHealthQueue:
				healthWriter <- h
			case s := <-agentStatusQueue:
				statusWriter <- s
			}
		}
	}()
}

type HealthStatusWriter struct {
	kv      storage.KeyValueStore
	healthC chan health.HealthUpdate
	statusC chan health.StatusUpdate
}

var _ health.HealthStatusUpdateWriter = (*HealthStatusWriter)(nil)

// Writes health and status updates to the keys "health" and "status" at the
// root of the provided KeyValueStore. To write updates to a specific path,
// use a prefixed KeyValueStore.
func NewHealthStatusWriter(ctx context.Context, kv storage.KeyValueStore, lg *zap.SugaredLogger) health.HealthStatusUpdateWriter {
	healthC := make(chan health.HealthUpdate, 8)
	statusC := make(chan health.StatusUpdate, 8)
	go func() {
		for h := range healthC {
			bytes, err := protojson.Marshal(h.Health)
			if err != nil {
				continue
			}
			if err := kv.Put(ctx, "health", bytes); err != nil {
				lg.With(
					zap.Error(err),
					zap.String("id", h.ID),
				).Error("failed to write health update to key-value store")
			}
		}
	}()
	go func() {
		for s := range statusC {
			bytes, err := protojson.Marshal(s.Status)
			if err != nil {
				continue
			}
			if err := kv.Put(ctx, "status", bytes); err != nil {
				lg.With(
					zap.Error(err),
					zap.String("id", s.ID),
				).Error("failed to write status update to key-value store")
			}
		}
	}()
	return &HealthStatusWriter{
		kv:      kv,
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
	events, err := kv.Watch(ctx, "", storage.WithPrefix())
	if err != nil {
		return nil, err
	}

	decodeKey := func(key string) (string, string, bool) {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			return "", "", false
		}
		return parts[0], parts[2], true
	}

	go func() {
		defer close(healthC)
		defer close(statusC)

		for event := range events {
			switch event.EventType {
			case storage.WatchEventPut:
				id, kind, ok := decodeKey(event.Current.Key())
				if !ok || !options.filter(id) {
					continue
				}

				switch kind {
				case "health":
					msg := &corev1.Health{}
					if err := protojson.Unmarshal(event.Current.Value(), msg); err != nil {
						continue
					}
					healthC <- health.HealthUpdate{
						ID:     id,
						Health: msg,
					}
				case "status":
					msg := &corev1.Status{}
					if err := protojson.Unmarshal(event.Current.Value(), msg); err != nil {
						continue
					}
					statusC <- health.StatusUpdate{
						ID:     id,
						Status: msg,
					}
				}
			case storage.WatchEventDelete:
				id, kind, ok := decodeKey(event.Previous.Key())
				if !ok || !options.filter(id) {
					continue
				}

				switch kind {
				case "health":
					healthC <- health.HealthUpdate{ID: id}
				case "status":
					statusC <- health.StatusUpdate{ID: id}
				}
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
