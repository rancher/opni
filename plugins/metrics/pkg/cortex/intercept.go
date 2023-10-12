package cortex

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"errors"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type WriteHandlerFunc = func(context.Context, *cortexpb.WriteRequest, ...grpc.CallOption) (*cortexpb.WriteResponse, error)

type RequestInterceptor interface {
	Intercept(ctx context.Context, req *cortexpb.WriteRequest, handler WriteHandlerFunc) (*cortexpb.WriteResponse, error)
}

type federatingInterceptor struct {
	conf InterceptorConfig

	mdTracker *metadataTracker
}

type InterceptorConfig struct {
	// The label to use as the tenant ID
	IdLabelName string
}

func NewFederatingInterceptor(conf InterceptorConfig) RequestInterceptor {
	return &federatingInterceptor{
		conf:      conf,
		mdTracker: newMetadataTracker(),
	}
}

var emptyResponse = &cortexpb.WriteResponse{}

const (
	lowerOrgIDHeaderName = "x-scope-orgid"
	underscoreName       = "__name__"
)

// Intercept takes a write request and splits it into multiple requests, grouped
// by tenant id found within the configured label name.
// The outgoingCtx must have an org id set, as well as outgoing metadata, to
// use as the default tenant id for timeseries without the configured label.
func (t *federatingInterceptor) Intercept(
	outgoingCtx context.Context, req *cortexpb.WriteRequest,
	handler WriteHandlerFunc,
) (*cortexpb.WriteResponse, error) {
	if len(req.Metadata) > 0 {
		// Metadata comes in periodically, separate from timeseries, and in one
		// large batch. Because we are splitting timeseries samples to other
		// tenants, we also need to replicate the metadata to those tenants.
		// We need to keep track of names of metrics for which we have sent
		// timeseries data to, so that we can send metadata for those metrics
		// to the same tenants the next time metadata comes in.
		var errs []error
		_, err := handler(outgoingCtx, req)
		if err != nil {
			errs = append(errs, err)
		}
		errs = append(errs, t.mdTracker.replicate(req.Metadata, func(tenantId string, metricMetadata []*cortexpb.MetricMetadata) error {
			req.Metadata = metricMetadata
			md, _, _ := metadata.FromOutgoingContextRaw(outgoingCtx)
			md[lowerOrgIDHeaderName] = []string{tenantId}
			if _, err := handler(outgoingCtx, req); err != nil {
				return err
			}
			return nil
		})...)
		return nil, errors.Join(errs...)
	}
	// for each time series in the request, check if there is a matching tenant ID label.
	// if there is, start a new request for this and all subsequent entries with that tenant ID.
	defaultId, err := user.ExtractOrgID(outgoingCtx)
	if err != nil {
		panic(err)
	}

	record := true
	startTime := time.Now()

	// save the original slice (header) over the whole timeseries array,
	// so that we can restore it after modifying req.Timeseries later on.
	allTimeseries := req.Timeseries
	var errs []error

	// partition the timeseries into buckets by tenant ID. The first partition is
	// the default tenant ID, which is used for timeseries without the configured
	// label. Subsequent partitions are for federated tenants.
	PartitionById(reinterpretTimeseries(req.Timeseries),
		// note: check gc annotations to make sure this function is inlined
		func(idx int) string {
			labels := req.Timeseries[idx].Labels
			for i := len(labels) - 1; i >= 0; i-- {
				if labels[i].Name == t.conf.IdLabelName {
					req.Timeseries[idx].Labels = append(req.Timeseries[idx].Labels[:i], req.Timeseries[idx].Labels[i+1:]...)
					return labels[i].Value
				}
			}
			return ""
		},
		func(id string, start, end int) {
			if record {
				record = false
				latencyPerTimeseries := time.Since(startTime) / time.Duration(len(req.Timeseries))
				hRemoteWriteProcessingLatency.Record(outgoingCtx, latencyPerTimeseries.Nanoseconds())
				cRemoteWriteTotalProcessedSeries.Add(outgoingCtx, int64(len(req.Timeseries)))
			}
			req.Timeseries = allTimeseries[start:end] // reuse the same request
			md, _, _ := metadata.FromOutgoingContextRaw(outgoingCtx)
			if id == "" {
				id = defaultId
			} else {
				for _, ts := range req.Timeseries {
					for _, l := range ts.Labels {
						if l.Name == underscoreName {
							t.mdTracker.track(l.Value, id)
							break
						}
					}
				}
			}
			if len(md[lowerOrgIDHeaderName]) == 1 {
				md[lowerOrgIDHeaderName][0] = id
			} else {
				md[lowerOrgIDHeaderName] = []string{id}
			}
			if _, err := handler(outgoingCtx, req); err != nil {
				errs = append(errs, err)
			}
		},
	)

	// restore the complete slice to be reused by the caller
	req.Timeseries = allTimeseries

	if len(errs) > 0 {
		// if there was one error, return it
		if len(errs) == 1 {
			return nil, errs[0]
		}
		// it is likely that all errors are the same, and we can only return one
		// error, so return the error with the largest code. The only actual
		// scenario we want to catch here is if one of the errors is an http
		// status code, in which case it will always be the chosen error.
		//
		toReturn := status.Convert(errs[0])
		for _, err := range errs[1:] {
			if statErr := status.Convert(err); statErr.Code() > toReturn.Code() {
				toReturn = statErr
			}
		}
		return nil, toReturn.Err()
	}

	return emptyResponse, nil
}

type PassthroughInterceptor struct{}

func (t *PassthroughInterceptor) Intercept(
	ctx context.Context, req *cortexpb.WriteRequest,
	handler WriteHandlerFunc,
) (*cortexpb.WriteResponse, error) {
	return handler(ctx, req)
}

func (t *PassthroughInterceptor) InterceptSlow(
	ctx context.Context, req *cortexpb.WriteRequest,
	handler WriteHandlerFunc,
) (*cortexpb.WriteResponse, error) {
	return handler(ctx, req)
}

// tracks metric names for which corresponding metadata must be replicated,
// and to which tenants.
type metadataTracker struct {
	mu                   sync.RWMutex
	tenantTrackedMetrics map[string]map[string]struct{}
}

func newMetadataTracker() *metadataTracker {
	return &metadataTracker{
		tenantTrackedMetrics: make(map[string]map[string]struct{}),
	}
}

func (mt *metadataTracker) replicate(metricMetadata []*cortexpb.MetricMetadata, replicateFn func(tenantId string, metricMetadata []*cortexpb.MetricMetadata) error) []error {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	replications := make(map[string][]*cortexpb.MetricMetadata)
	for _, md := range metricMetadata {
		if tenantIds, ok := mt.tenantTrackedMetrics[md.MetricFamilyName]; ok {
			for tenantId := range tenantIds {
				replications[tenantId] = append(replications[tenantId], md)
			}
		}
	}
	var errs []error
	for tenantId, metricMetadata := range replications {
		if err := replicateFn(tenantId, metricMetadata); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (mt *metadataTracker) track(metricName, tenantId string) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if tenants, ok := mt.tenantTrackedMetrics[metricName]; ok {
		tenants[tenantId] = struct{}{}
	} else {
		mt.tenantTrackedMetrics[metricName] = map[string]struct{}{tenantId: {}}
	}
}

func reinterpretTimeseries(ts []cortexpb.PreallocTimeseries) []*cortexpb.TimeSeries {
	return *(*[]*cortexpb.TimeSeries)(unsafe.Pointer(&ts))
}

type Partition struct {
	Id         string
	Start, End int
}

var partitionPool = sync.Pool{
	New: func() interface{} {
		return &Partition{}
	},
}

var partitionSlicePool = sync.Pool{
	New: func() interface{} {
		p := make([]*Partition, 0, 32)
		return &p
	},
}

var lookupTablePool = sync.Pool{
	New: func() interface{} {
		return make(map[string]int, 32)
	},
}

func reusePartition(part *Partition) {
	part.Id = ""
	part.Start = 0
	part.End = 0
	partitionPool.Put(part)
}

func reusePartitionSlice(parts *[]*Partition) {
	*parts = (*parts)[:0]
	partitionSlicePool.Put(parts)
}

func reuseLookupTable(table map[string]int) {
	for k := range table {
		delete(table, k)
	}
	lookupTablePool.Put(table)
}

// PartitionById arranges the elements of a slice into any number of partitions,
// where each partition contains elements grouped by a common ID. This function
// returns a list of partitions, where each has a start and end index and
// corresponding ID. The partition slice returned from this function should be
// reused by calling ReusePartitionSlice.
//
//	The order of partitions (by ID) are as follows:
//	1. The first partition ID is the ID at index 0.
//	2. Subsequent partition IDs are in the inverse order of appearance when
//	   iterating backwards through the array.
//
//	For example:
//	  IDs:   A | D B A C A D A D E D D E B D E F C G D H A A H J D C C C I A
//	Index:   0 |                         9   8 7   6         5 4 3     2 1
//	           | -> partition order                   order of appearance <-
//	Partition Order: A B E F G H J D C I
func PartitionById[T any](arr []*T, idForElement func(int) string, callback func(id string, start, end int)) {
	if len(arr) == 0 {
		return
	}
	firstId := idForElement(0)
	partitions := partitionSlicePool.Get().(*[]*Partition)
	first := Partition{
		Id:    firstId,
		Start: 0,
		End:   len(arr),
	}
	lookup := lookupTablePool.Get().(map[string]int)

	// loop backwards to improve locality for swaps
	for i := len(arr) - 1; i >= 0; i-- {
		tenantId := idForElement(i)
		if tenantId == firstId {
			continue
		}

		if i != first.End-1 {
			// swap the element to the end of the first partition
			ptrswap(&arr[i], &arr[first.End-1])
		}

		revIdx, ok := lookup[tenantId]
		if !ok {
			first.End--
			// create a new partition
			newPartition := partitionPool.Get().(*Partition)
			newPartition.Id = tenantId
			newPartition.Start = first.End
			newPartition.End = first.End + 1

			// move the last element of the first partition to the new partition
			lookup[tenantId] = len(*partitions)
			*partitions = append(*partitions, newPartition)
		} else {
			// append to existing partition by swapping the element to the end of
			// each partition and shifting the boundary one element to the left

			// start by swapping from the first partition into the most recently added
			// partition (which is closest to the end of the first partition)
			last := (*partitions)[len((*partitions))-1]
			ptrswap(&arr[first.End-1], &arr[last.End-1])
			first.End--
			last.Start--

			// then continue from the most recently added partition to the target
			for p := len(*partitions) - 1; p > revIdx; p-- {
				cur := (*partitions)[p]
				next := (*partitions)[p-1]
				ptrswap(&arr[(*partitions)[p].End-1], &arr[next.End-1])
				cur.End--
				next.Start--
			}
		}
	}
	callback(first.Id, first.Start, first.End)
	for i := len((*partitions)) - 1; i >= 0; i-- {
		part := (*partitions)[i]
		callback(part.Id, part.Start, part.End)
		reusePartition(part)
	}
	reuseLookupTable(lookup)
	reusePartitionSlice(partitions)
}

func ptrswap[T any](a, b **T) {
	*a, *b = *b, *a
}
