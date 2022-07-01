package health

import (
	"context"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
)

type Monitor struct {
	MonitorOptions
	mu            sync.Mutex
	currentHealth map[string]*corev1.Health
	currentStatus map[string]*corev1.Status
}

type MonitorOptions struct {
	lg *zap.SugaredLogger
}

type MonitorOption func(*MonitorOptions)

func (o *MonitorOptions) apply(opts ...MonitorOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLogger(lg *zap.SugaredLogger) MonitorOption {
	return func(o *MonitorOptions) {
		o.lg = lg
	}
}

func NewMonitor(opts ...MonitorOption) *Monitor {
	options := MonitorOptions{
		lg: zap.NewNop().Sugar(),
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
			).Info("received health update")
			m.currentHealth[update.ID] = update.Health
			m.mu.Unlock()
		case update, ok := <-updater.StatusC():
			if !ok {
				m.lg.Debug("status update channel closed")
				return
			}
			m.mu.Lock()
			m.lg.With(
				"id", update.ID,
				"connected", update.Status.Connected,
			).Info("received status update")
			m.currentStatus[update.ID] = update.Status
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
