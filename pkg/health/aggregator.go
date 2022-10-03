package health

import (
	"context"
	"fmt"
	"sort"
	"sync"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Aggregator implements a HealthServer that queries one or more HealthClients
// and aggregates the results.
// This server will report as ready if and only if all clients report as ready.
type Aggregator struct {
	controlv1.UnsafeHealthServer
	AggregatorOptions

	clientsMu sync.RWMutex
	clients   map[string]controlv1.HealthClient
}

type AggregatorOptions struct {
	staticAnnotations map[string]string
}

type AggregatorOption func(*AggregatorOptions)

func (o *AggregatorOptions) apply(opts ...AggregatorOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithStaticAnnotations(staticAnnotations map[string]string) AggregatorOption {
	return func(o *AggregatorOptions) {
		o.staticAnnotations = staticAnnotations
	}
}

func NewAggregator(opts ...AggregatorOption) *Aggregator {
	options := &AggregatorOptions{}
	options.apply(opts...)

	return &Aggregator{
		clients: make(map[string]controlv1.HealthClient),
	}
}

func (h *Aggregator) AddClient(name string, client controlv1.HealthClient) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	h.clients[name] = client
}

func (h *Aggregator) RemoveClient(name string) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	delete(h.clients, name)
}

func (h *Aggregator) GetHealth(ctx context.Context, _ *emptypb.Empty) (*corev1.Health, error) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	clientConditions := make([][]string, len(h.clients))
	clientsReady := make([]bool, len(h.clients))

	var wg sync.WaitGroup
	wg.Add(len(h.clients))
	i := 0
	for name, client := range h.clients {
		name, client := name, client
		go func(i int) {
			defer wg.Done()
			health, err := client.GetHealth(ctx, &emptypb.Empty{})
			if err != nil {
				switch status.Code(err) {
				case codes.Unavailable:
					clientConditions[i] = []string{"%s is unavailable", name}
				case codes.ResourceExhausted:
					clientConditions[i] = []string{"%s is overloaded", name}
				case codes.DeadlineExceeded, codes.Canceled:
					clientConditions[i] = []string{"%s timed out", name}
				default:
					clientConditions[i] = []string{fmt.Sprintf("%s: %s", name, err.Error())}
				}
				return
			}
			clientConditions[i] = health.Conditions
			clientsReady[i] = health.Ready
		}(i)
		i++
	}
	wg.Wait()

	allClientsReady := true
	for _, ready := range clientsReady {
		allClientsReady = allClientsReady && ready
	}
	allConditions := lo.Flatten(clientConditions)
	sort.Strings(allConditions)
	return &corev1.Health{
		Ready:       allClientsReady,
		Conditions:  allConditions,
		Annotations: h.staticAnnotations,
	}, nil
}
