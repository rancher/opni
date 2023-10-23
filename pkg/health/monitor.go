package health

import (
	"context"
	"sync"

	"log/slog"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health/annotations"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/util"
)

type Monitor struct {
	MonitorOptions
	mu              sync.Mutex
	currentHealth   map[string]*corev1.Health
	currentStatus   map[string]*corev1.Status
	healthListeners []chan *corev1.ClusterHealth
	statusListeners []chan *corev1.ClusterStatus
}

type MonitorOptions struct {
	lg *slog.Logger
}

type MonitorOption func(*MonitorOptions)

func (o *MonitorOptions) apply(opts ...MonitorOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLogger(lg *slog.Logger) MonitorOption {
	return func(o *MonitorOptions) {
		o.lg = lg
	}
}

func NewMonitor(opts ...MonitorOption) *Monitor {
	options := MonitorOptions{
		lg: logger.NewNop(),
	}
	options.apply(opts...)

	return &Monitor{
		MonitorOptions: options,
		currentHealth:  make(map[string]*corev1.Health),
		currentStatus:  make(map[string]*corev1.Status),
	}
}

func (m *Monitor) Run(ctx context.Context, updater HealthStatusUpdater) {
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.currentHealth = make(map[string]*corev1.Health)
		m.currentStatus = make(map[string]*corev1.Status)
		for _, l := range m.healthListeners {
			close(l)
		}
		for _, s := range m.statusListeners {
			close(s)
		}
		m.healthListeners = nil
		m.statusListeners = nil
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updater.HealthC():
			if !ok {
				m.lg.Debug("health update channel closed")
				return
			}
			m.mu.Lock()
			m.lg.With(
				"id", update.ID,
				"ready", update.Health.Ready,
				"conditions", update.Health.Conditions,
			).With(
				annotations.KeyValuePairs(update.Health.GetAnnotations())...,
			).Info("received health update")
			m.currentHealth[update.ID] = update.Health
			for _, ch := range m.healthListeners {
				ch <- &corev1.ClusterHealth{
					Cluster: &corev1.Reference{
						Id: update.ID,
					},
					Health: util.ProtoClone(update.Health),
				}
			}
			m.mu.Unlock()
		case update, ok := <-updater.StatusC():
			if !ok {
				m.lg.Debug("status update channel closed")
				return
			}
			m.mu.Lock()
			updateLog := m.lg.With(
				"id", update.ID,
				"connected", update.Status.Connected,
			)
			if len(update.Status.SessionAttributes) > 0 {
				updateLog = updateLog.With("attributes", update.Status.SessionAttributes)
			}
			updateLog.Info("received status update")
			m.currentStatus[update.ID] = update.Status
			for _, ch := range m.statusListeners {
				ch <- &corev1.ClusterStatus{
					Cluster: &corev1.Reference{
						Id: update.ID,
					},
					Status: util.ProtoClone(update.Status),
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *Monitor) GetHealthStatus(id string) *corev1.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &corev1.HealthStatus{
		Health: util.ProtoClone(m.currentHealth[id]),
		Status: util.ProtoClone(m.currentStatus[id]),
	}
}

func (m *Monitor) WatchHealthStatus(ctx context.Context) <-chan *corev1.ClusterHealthStatus {
	m.mu.Lock()
	ch := make(chan *corev1.ClusterHealthStatus, 100)
	hl := make(chan *corev1.ClusterHealth, 10)
	sl := make(chan *corev1.ClusterStatus, 10)
	m.healthListeners = append(m.healthListeners, hl)
	m.statusListeners = append(m.statusListeners, sl)

	curHealth := make(map[string]*corev1.Health, len(m.currentHealth))
	curStatus := make(map[string]*corev1.Status, len(m.currentStatus))
	for k, v := range m.currentHealth {
		curHealth[k] = util.ProtoClone(v)
	}
	for k, v := range m.currentStatus {
		curStatus[k] = util.ProtoClone(v)
	}
	m.mu.Unlock()

	go func() {
		for id, h := range curHealth {
			s := curStatus[id]
			ch <- &corev1.ClusterHealthStatus{
				Cluster: &corev1.Reference{
					Id: id,
				},
				HealthStatus: &corev1.HealthStatus{
					Health: util.ProtoClone(h),
					Status: util.ProtoClone(s),
				},
			}
		}
	LOOP:
		for {
			var ref *corev1.Reference
			select {
			case <-ctx.Done():
				break LOOP
			case health, ok := <-hl:
				if !ok {
					break LOOP
				}
				ref = health.Cluster
				curHealth[ref.Id] = health.Health
			case status, ok := <-sl:
				if !ok {
					break LOOP
				}
				ref = status.Cluster
				curStatus[ref.Id] = status.Status
			}
			hs := &corev1.ClusterHealthStatus{
				Cluster: ref,
				HealthStatus: &corev1.HealthStatus{
					Health: util.ProtoClone(curHealth[ref.Id]),
					Status: util.ProtoClone(curStatus[ref.Id]),
				},
			}
			select {
			case <-ctx.Done():
			case ch <- hs:
			}
		}

		m.mu.Lock()
		defer m.mu.Unlock()
		for i, h := range m.healthListeners {
			if h == hl {
				m.healthListeners = append(m.healthListeners[:i], m.healthListeners[i+1:]...)
				close(hl)
				break
			}
		}
		for i, s := range m.statusListeners {
			if s == sl {
				m.statusListeners = append(m.statusListeners[:i], m.statusListeners[i+1:]...)
				close(sl)
				break
			}
		}
	}()

	return ch
}
