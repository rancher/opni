package health

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Listener struct {
	ListenerOptions
	statusUpdate chan StatusUpdate
	healthUpdate chan HealthUpdate
	idLocksMu    sync.Mutex
	idLocks      map[string]*sync.Mutex
	closed       chan struct{}
	sem          *semaphore.Weighted
}

type ListenerOptions struct {
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
		ListenerOptions: options,
		statusUpdate:    make(chan StatusUpdate, options.updateQueueCap),
		healthUpdate:    make(chan HealthUpdate, options.updateQueueCap),
		idLocks:         make(map[string]*sync.Mutex),
		closed:          make(chan struct{}),
		sem:             semaphore.NewWeighted(options.maxConnections),
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
