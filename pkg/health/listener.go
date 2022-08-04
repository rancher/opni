package health

import (
	"context"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/alerting"
	"github.com/rancher/opni/pkg/alerting/condition"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util"
	ap "github.com/rancher/opni/plugins/alerting/pkg/alerting"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Listener struct {
	ListenerOptions
	statusUpdate        chan StatusUpdate
	healthUpdate        chan HealthUpdate
	idLocksMu           sync.Mutex
	idLocks             map[string]*sync.Mutex
	closed              chan struct{}
	sem                 *semaphore.Weighted
	AlertProvider       *alerting.Provider
	alertToggle         chan struct{}
	alertTickerDuration time.Duration
	alertCondition      *alertingv1alpha.AlertCondition
}

type ListenerOptions struct {
	alertProvider  *alerting.Provider
	alertToggle    chan struct{}
	tickerDuration time.Duration
	alertCondition *alertingv1alpha.AlertCondition
	interval       time.Duration
	maxJitter      time.Duration
	maxConnections int64
	updateQueueCap int
}

type ListenerOption func(*ListenerOptions)

func (o *ListenerOptions) apply(opts ...ListenerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithPollInterval(interval time.Duration) ListenerOption {
	return func(o *ListenerOptions) {
		o.interval = interval
	}
}

func WithMaxJitter(duration time.Duration) ListenerOption {
	return func(o *ListenerOptions) {
		o.maxJitter = duration
	}
}

// Max concurrent connections. Defaults to math.MaxInt64
func WithMaxConnections(max int64) ListenerOption {
	return func(o *ListenerOptions) {
		o.maxConnections = max
	}
}

// Max concurrent queued updates before blocking. Defaults to 1000
func WithUpdateQueueCap(cap int) ListenerOption {
	return func(o *ListenerOptions) {
		o.updateQueueCap = cap
	}
}

func WithAlertToggle() ListenerOption {
	return func(o *ListenerOptions) {
		o.alertToggle = make(chan struct{})
	}
}

func WithDisconnectTimeout(timeout time.Duration) ListenerOption {
	_, isSet := os.LookupEnv(ap.LocalBackendEnvToggle)
	if isSet {
		return func(o *ListenerOptions) {
			o.tickerDuration = time.Millisecond * 100
		}
	}
	return func(o *ListenerOptions) {
		o.tickerDuration = timeout
	}
}

func WithDefaultAlertCondition() ListenerOption {
	return func(o *ListenerOptions) {
		o.alertCondition = condition.OpniDisconnect
	}
}

func WithAlertProvider(alertProvider *alerting.Provider) ListenerOption {
	return func(o *ListenerOptions) {
		o.alertProvider = alertProvider
	}
}

func NewListener(opts ...ListenerOption) *Listener {
	options := ListenerOptions{
		interval:       5 * time.Second,
		maxJitter:      1 * time.Second,
		updateQueueCap: 1000,
		maxConnections: math.MaxInt64,
	}
	options.apply(opts...)
	if options.maxJitter > options.interval {
		options.maxJitter = options.interval
	}
	return &Listener{
		AlertProvider:       options.alertProvider,
		alertToggle:         options.alertToggle,
		alertCondition:      options.alertCondition,
		alertTickerDuration: options.tickerDuration,
		ListenerOptions:     options,
		statusUpdate:        make(chan StatusUpdate, options.updateQueueCap),
		healthUpdate:        make(chan HealthUpdate, options.updateQueueCap),
		idLocks:             make(map[string]*sync.Mutex),
		closed:              make(chan struct{}),
		sem:                 semaphore.NewWeighted(options.maxConnections),
	}
}

func (l *Listener) HandleConnection(ctx context.Context, clientset HealthClientSet) {
	// if the listener is closed, exit immediately
	select {
	case <-l.closed:
		return
	default:
	}

	if err := l.sem.Acquire(ctx, 1); err != nil {
		return // context canceled
	}
	defer l.sem.Release(1)

	id := cluster.StreamAuthorizedID(ctx)

	// if we are setting an alert condition for disconnections
	if l.alertProvider != nil {
		l.AlertDisconnectLoop(id)
	}

	l.idLocksMu.Lock()
	var clientLock *sync.Mutex
	if m, ok := l.idLocks[id]; !ok {
		clientLock = &sync.Mutex{}
		l.idLocks[id] = clientLock
	} else {
		clientLock = m
	}
	l.idLocksMu.Unlock()

	// locks keyed based on agent id ensure this function is reentrant during
	// agent reconnects
	clientLock.Lock()
	defer clientLock.Unlock() // 3rd

	l.statusUpdate <- StatusUpdate{
		ID: id,
		Status: &corev1.Status{
			Connected: true,
		},
	}
	defer func() { // 2nd
		l.healthUpdate <- HealthUpdate{
			ID:     id,
			Health: &corev1.Health{},
		}
		l.statusUpdate <- StatusUpdate{
			ID: id,
			Status: &corev1.Status{
				Connected: false,
			},
		}
	}()
	curHealth, err := clientset.GetHealth(ctx, &emptypb.Empty{})
	if err == nil {
		l.healthUpdate <- HealthUpdate{
			ID:     id,
			Health: util.ProtoClone(curHealth),
		}
	}

	calcDuration := func() time.Duration {
		var jitter time.Duration
		if l.maxJitter > 0 {
			jitter = time.Duration(rand.Int63n(int64(l.maxJitter)))
		}
		return l.interval + jitter
	}

	timer := time.NewTimer(calcDuration())
	defer timer.Stop() // 1st
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.closed:
			return
		case <-timer.C:
			health, err := clientset.GetHealth(ctx, &emptypb.Empty{})
			if err == nil {
				if !proto.Equal(health, curHealth) {
					curHealth = health
					l.healthUpdate <- HealthUpdate{
						ID:     id,
						Health: util.ProtoClone(curHealth),
					}
				}
			}
			timer.Reset(calcDuration())
		}
	}
}

func (l *Listener) AlertDisconnectLoop(agentId string) {
	ctx := context.Background()
	if l.alertToggle == nil {
		l.alertToggle = make(chan struct{})
	}
	if l.alertCondition == nil {
		l.alertCondition = condition.OpniDisconnect
	}

	// prevent data race
	alertConditionTemplateCopy := proto.Clone(l.alertCondition).(*alertingv1alpha.AlertCondition)

	// Eventually replace with templates : Waiting on AlertManager Routes backend
	alertConditionTemplateCopy.Name = strings.Replace(
		l.alertCondition.Name,
		"{{ .agentId }}",
		agentId, -1)
	alertConditionTemplateCopy.Description = strings.Replace(
		l.alertCondition.Description,
		"{{ .agentId }}",
		agentId, -1)

	alertConditionTemplateCopy.Description = strings.Replace(
		l.alertCondition.Description, "{{ .timeout }}",
		l.alertTickerDuration.String(), -1)

	go func() {
		id, err := (*l.alertProvider).CreateAlertCondition(ctx, l.alertCondition)
		retryOnFailure := time.NewTicker(time.Second)

		for err != nil {
			select {
			case <-retryOnFailure.C:
				retryOnFailure = time.NewTicker(time.Second)
				id, err = (*l.alertProvider).CreateAlertCondition(ctx, l.alertCondition)
			case <-l.closed:
				retryOnFailure.Stop()
				return
			}
		}
		if l.alertTickerDuration < 0 {
			l.alertTickerDuration = time.Second * 60
		}
		ticker := time.NewTicker(l.alertTickerDuration)

		for {
			select {
			case <-ticker.C: // received no message from agent in the entier duration
				_, err = (*l.alertProvider).TriggerAlerts(ctx, &alertingv1alpha.TriggerAlertsRequest{
					Id: id,
				})
				if err != nil {
					ticker = time.NewTicker(time.Second) // retry trigger more often
				} else {
					ticker = time.NewTicker(l.alertTickerDuration)
				}

			case <-l.alertToggle: // received a message from agent
				ticker.Stop()
				ticker = time.NewTicker(l.alertTickerDuration)

			case <-l.closed: // listener is closed, stop
				ticker.Stop()
				go func() {
					(*l.alertProvider).DeleteAlertCondition(ctx, id)
				}()
				return
			}

		}
	}()
}

func (l *Listener) StatusC() chan StatusUpdate {
	return l.statusUpdate
}

func (l *Listener) HealthC() chan HealthUpdate {
	return l.healthUpdate
}

func (l *Listener) Close() {
	// Prevent any new connections from being handled
	close(l.closed)
	go func() {
		// Wait for all existing active handlers to exit
		l.sem.Acquire(context.Background(), l.maxConnections)
		// Close the health and status channels
		close(l.statusUpdate)
		close(l.healthUpdate)
	}()
}

// Implements gateway.ConnectionHandler
func (l *Listener) HandleAgentConnection(ctx context.Context, clientset agent.ClientSet) {
	l.HandleConnection(ctx, clientset)
}
