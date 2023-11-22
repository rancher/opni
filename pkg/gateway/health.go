package gateway

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"strings"
	sync "sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HealthStatusWriterManager struct {
	ctx                context.Context
	healthQueue        chan health.HealthUpdate
	statusQueue        chan health.StatusUpdate
	queueLock          sync.Mutex
	healthQueueByAgent map[string]chan health.HealthUpdate
	statusQueueByAgent map[string]chan health.StatusUpdate
	connectionsKv      storage.KeyValueStore
	logger             *slog.Logger
}

var _ TrackedConnectionListener = (*HealthStatusWriterManager)(nil)

func NewHealthStatusWriterManager(ctx context.Context, rootKv storage.KeyValueStore, lg *slog.Logger) *HealthStatusWriterManager {
	healthQueue := make(chan health.HealthUpdate, 256)
	statusQueue := make(chan health.StatusUpdate, 256)
	mgr := &HealthStatusWriterManager{
		ctx:                ctx,
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
	w.logger.With("id", id).Debug("initializing agent health queue")
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
	w.logger.With("id", id).Debug("initializing agent status queue")
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
func (w *HealthStatusWriterManager) HandleTrackedConnection(ctx context.Context, agentId string, leaseId string, _ *corev1.InstanceInfo) {
	// see docs for NewHealthStatusWriter for details on context usage
	hsw := NewHealthStatusWriter(context.WithoutCancel(ctx), kvutil.WithMessageCodec[*corev1.InstanceInfo](kvutil.WithKey(w.connectionsKv, path.Join(agentId, leaseId))), w.logger)
	agentBuffer := health.AsBuffer(w.agentHealthQueue(agentId), w.agentStatusQueue(agentId))
	w.logger.With("id", agentId).Debug("handling new tracked agent connection")
	go func() {
		defer hsw.Close()
		health.Copy(ctx, hsw, agentBuffer)
	}()
}

type HealthStatusWriter struct {
	v       storage.ValueStoreT[*corev1.InstanceInfo]
	healthC chan health.HealthUpdate
	statusC chan health.StatusUpdate
	closed  chan struct{}
}

var _ health.HealthStatusUpdateWriter = (*HealthStatusWriter)(nil)

// Writes health and status updates to an InstanceInfo object in the provided
// value store.
//
// This writer only modifies the value if the key already exists. If it does not
// exist or is deleted, updates will be ignored until it is created.
//
// The context is used to make requests to the value store. It should not need
// to be canceled under normal operation. To ensure resources are released from
// the writer, call Close() which will block until all messages have been
// processed (or the context is canceled, in which case the writer may fail to
// write the final health and status updates to the value store)
func NewHealthStatusWriter(ctx context.Context, v storage.ValueStoreT[*corev1.InstanceInfo], lg *slog.Logger) health.HealthStatusUpdateWriter {
	healthC := make(chan health.HealthUpdate, 8)
	statusC := make(chan health.StatusUpdate, 8)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		recvHealth := func(h health.HealthUpdate) error {
			var rev int64
			ii, err := v.Get(ctx, storage.WithRevisionOut(&rev))
			if err != nil {
				if storage.IsNotFound(err) {
					lg.Debug("discarding health update for non-existent instance")
					return nil
				}
				if errors.Is(err, ctx.Err()) {
					return err
				}
				lg.With(logger.Err(err)).Error("failed to get instance info during health update")
				return nil
			}
			if ii == nil {
				ii = &corev1.InstanceInfo{}
			}
			ii.Health = h.Health
			if err := v.Put(ctx, ii, storage.WithRevision(rev)); err != nil {
				if storage.IsConflict(err) {
					lg.Debug("discarding health update for deleted instance")
					return nil
				}
				if errors.Is(err, ctx.Err()) {
					return err
				}
				lg.With(logger.Err(err)).Error("failed to put instance info during health update")
			}
			return nil
		}
		recvStatus := func(s health.StatusUpdate) error {
			var rev int64
			ii, err := v.Get(ctx, storage.WithRevisionOut(&rev))
			if err != nil {
				if storage.IsNotFound(err) {
					lg.Debug("discarding status update for non-existent instance")
					return nil
				}
				if errors.Is(err, ctx.Err()) {
					return err
				}
				lg.With(logger.Err(err)).Error("failed to get instance info during status update")
				return nil
			}
			if ii == nil {
				ii = &corev1.InstanceInfo{}
			}
			ii.Status = s.Status
			if err := v.Put(ctx, ii, storage.WithRevision(rev)); err != nil {
				if storage.IsConflict(err) {
					lg.Debug("discarding status update for deleted instance")
					return nil
				}
				if errors.Is(err, ctx.Err()) {
					return err
				}
				lg.With(logger.Err(err)).Error("failed to put instance info during status update")
			}
			return nil
		}
		for {
			select {
			case h, ok := <-healthC:
				if !ok {
					for h := range healthC {
						if err := recvHealth(h); err != nil {
							return
						}
					}
					return
				}
				if err := recvHealth(h); err != nil {
					return
				}
			case s, ok := <-statusC:
				if !ok {
					for s := range statusC {
						if err := recvStatus(s); err != nil {
							return
						}
					}
					return
				}
				if err := recvStatus(s); err != nil {
					return
				}
			}
		}
	}()

	return &HealthStatusWriter{
		v:       v,
		healthC: healthC,
		statusC: statusC,
		closed:  closed,
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
	<-w.closed
}

type HealthStatusReader struct {
	kv      storage.KeyValueStore
	healthC chan health.HealthUpdate
	statusC chan health.StatusUpdate
}

var _ health.HealthStatusUpdateReader = (*HealthStatusReader)(nil)

type HealthStatusReaderOptions struct {
}

type HealthStatusReaderOption func(*HealthStatusReaderOptions)

func (o *HealthStatusReaderOptions) apply(opts ...HealthStatusReaderOption) {
	for _, op := range opts {
		op(o)
	}
}

func NewHealthStatusReader(ctx context.Context, kv storage.KeyValueStore, opts ...HealthStatusReaderOption) (health.HealthStatusUpdateReader, error) {
	options := HealthStatusReaderOptions{}
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
				if info.Health != nil {
					healthC <- health.HealthUpdate{
						ID:     id,
						Health: info.Health,
					}
				}

				if info.Status != nil {
					statusC <- health.StatusUpdate{
						ID:     id,
						Status: info.Status,
					}
				}
			case storage.WatchEventDelete:
				id, _, ok := decodeKey(event.Previous.Key())
				if !ok {
					// fmt.Println("could not decode key")
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
				statusC <- health.StatusUpdate{
					ID: id,
					Status: &corev1.Status{
						Timestamp: timestamppb.Now(),
						Connected: false,
					},
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
