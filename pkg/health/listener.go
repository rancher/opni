package health

import (
	"context"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	agentv1 "github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/alerting"
	"github.com/rancher/opni/pkg/alerting/condition"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Listener struct {
	controlv1.UnsafeHealthListenerServer
	ListenerOptions
	statusUpdate            chan StatusUpdate
	healthUpdate            chan HealthUpdate
	idLocksMu               sync.Mutex
	idLocks                 map[string]*sync.Mutex
	incomingHealthUpdatesMu sync.RWMutex
	incomingHealthUpdates   map[string]chan HealthUpdate
	closed                  chan struct{}
	sem                     *semaphore.Weighted
	AlertProvider           *alerting.Provider
	alertToggle             chan struct{}
	alertTickerDuration     time.Duration
	alertCondition          *alertingv1alpha.AlertCondition
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
	_, isSet := os.LookupEnv(shared.LocalBackendEnvToggle)
	if isSet {
		return func(o *ListenerOptions) {
			o.tickerDuration = time.Second * 60
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
		// poll slowly, health updates are also sent on demand by the agent
		interval:       30 * time.Second,
		maxJitter:      30 * time.Second,
		updateQueueCap: 1000,
		maxConnections: math.MaxInt64,
	}
	options.apply(opts...)
	if options.maxJitter > options.interval {
		options.maxJitter = options.interval
	}
	return &Listener{
		AlertProvider:         options.alertProvider,
		alertToggle:           options.alertToggle,
		alertCondition:        options.alertCondition,
		alertTickerDuration:   options.tickerDuration,
		ListenerOptions:       options,
		statusUpdate:          make(chan StatusUpdate, options.updateQueueCap),
		healthUpdate:          make(chan HealthUpdate, options.updateQueueCap),
		incomingHealthUpdates: make(map[string]chan HealthUpdate),
		idLocks:               make(map[string]*sync.Mutex),
		closed:                make(chan struct{}),
		sem:                   semaphore.NewWeighted(options.maxConnections),
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
	defer l.sem.Release(1) // 5th

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
	defer clientLock.Unlock() // 4th

	l.incomingHealthUpdatesMu.Lock()
	incomingHealthUpdates := make(chan HealthUpdate, 10)
	l.incomingHealthUpdates[id] = incomingHealthUpdates
	l.incomingHealthUpdatesMu.Unlock()

	defer func() { // 3rd
		l.incomingHealthUpdatesMu.Lock()
		delete(l.incomingHealthUpdates, id)
		l.incomingHealthUpdatesMu.Unlock()
	}()

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
				healthEq := health.Equal(curHealth)
				healthNewer := health.NewerThan(curHealth)
				if !healthEq && healthNewer {
					curHealth = health
					l.healthUpdate <- HealthUpdate{
						ID:     id,
						Health: util.ProtoClone(curHealth),
					}
				}
			}
			timer.Reset(calcDuration())
		case up := <-incomingHealthUpdates:
			health, id := up.Health, up.ID
			healthEq := health.Equal(curHealth)
			healthNewer := health.NewerThan(curHealth)
			if !healthEq && healthNewer {
				curHealth = util.ProtoClone(health)
				l.healthUpdate <- HealthUpdate{
					ID:     id,
					Health: util.ProtoClone(curHealth),
				}
			}

			// drain the timer channel if it happened to fire at the same time
			if !timer.Stop() {
				<-timer.C
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
		alertConditionTemplateCopy.Description,
		"{{ .agentId }}",
		agentId, -1)

	alertConditionTemplateCopy.Description = strings.Replace(
		alertConditionTemplateCopy.Description, "{{ .timeout }}",
		l.alertTickerDuration.String(), -1)

	go func() {
		id, err := (*l.alertProvider).CreateAlertCondition(ctx, alertConditionTemplateCopy)
		retryOnFailure := time.NewTicker(time.Second)
		defer retryOnFailure.Stop()
		for err != nil {
			select {
			case <-retryOnFailure.C:
				retryOnFailure.Reset(time.Second)
				id, err = alerting.DoCreate(*l.alertProvider, ctx, alertConditionTemplateCopy)
			case <-l.closed:
				return
			}
		}
		if l.alertTickerDuration <= 0 {
			l.alertTickerDuration = time.Second * 60
		}
		ticker := time.NewTicker(l.alertTickerDuration)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C: // received no message from agent in the entire duration
				_, err = alerting.DoTrigger(*l.alertProvider, ctx, &alertingv1alpha.TriggerAlertsRequest{
					ConditionId: id,
				})
				if err != nil {
					ticker.Reset(time.Second) // retry trigger more often
				} else {
					ticker.Reset(l.alertTickerDuration)
				}

			case <-l.alertToggle: // received a message from agent
				ticker.Reset(l.alertTickerDuration)

			case <-l.closed: // listener is closed, stop
				go func() {
					alerting.DoDelete(*l.alertProvider, ctx, id)
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
func (l *Listener) HandleAgentConnection(ctx context.Context, clientset agentv1.ClientSet) {
	l.HandleConnection(ctx, clientset)
}

// Implements controlv1.HealthListenerServer
func (l *Listener) UpdateHealth(ctx context.Context, req *corev1.Health) (*emptypb.Empty, error) {
	id := cluster.StreamAuthorizedID(ctx)

	l.incomingHealthUpdatesMu.RLock()
	defer l.incomingHealthUpdatesMu.RUnlock()

	if ch, ok := l.incomingHealthUpdates[id]; ok {
		ch <- HealthUpdate{
			ID:     id,
			Health: util.ProtoClone(req),
		}
	}

	return &emptypb.Empty{}, nil
}
