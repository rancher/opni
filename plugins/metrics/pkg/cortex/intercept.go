package cortex

import (
	"context"

	"errors"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const lowerOrgIDHeaderName = "x-scope-orgid"

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
	// Whether to drop the tenant ID label in the output
	DropIdLabel bool
}

func NewFederatingInterceptor(conf InterceptorConfig) RequestInterceptor {
	return federatingInterceptor{
		conf:      conf,
		mdTracker: newMetadataTracker(),
	}
}

// Intercept takes a write request and splits it into multiple requests, grouped
// by tenant id found within the configured label name.
// The outgoingCtx must have an org id set, as well as outgoing metadata, to
// use as the default tenant id for timeseries without the configured label.
func (t federatingInterceptor) Intercept(
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
	var buckets = map[string][]cortexpb.PreallocTimeseries{
		defaultId: {},
	}
TIMESERIES:
	for _, ts := range req.Timeseries {
		nameLabelIndex := -1
		for i, l := range ts.Labels {
			// in practice, this appears to always be index 0
			if l.Name == "__name__" {
				nameLabelIndex = i
				break
			}
		}
		if nameLabelIndex == -1 {
			continue
		}
		for i, l := range ts.Labels {
			if l.Name == t.conf.IdLabelName {
				if t.conf.DropIdLabel {
					ts.Labels = append(ts.Labels[:i], ts.Labels[i+1:]...)
				}
				buckets[l.Value] = append(buckets[l.Value], ts)
				t.mdTracker.track(ts.Labels[nameLabelIndex].Value, l.Value)
				continue TIMESERIES
			}
		}
		buckets[defaultId] = append(buckets[defaultId], ts)
	}

	// save the original slice so we can return it to the pool, along with all
	// the original timeseries entries
	allTimeseries := req.Timeseries

	// danger zone
	var errs []error
	for tenantId, series := range buckets {
		req.Timeseries = series // reuse the same request
		md, _, _ := metadata.FromOutgoingContextRaw(outgoingCtx)
		md[lowerOrgIDHeaderName] = []string{tenantId}
		if _, err := handler(outgoingCtx, req); err != nil {
			errs = append(errs, err)
		}
	}

	cortexpb.ReuseSlice(allTimeseries)

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
	return &cortexpb.WriteResponse{}, nil
}

type passthroughInterceptor struct{}

func (t passthroughInterceptor) Intercept(
	ctx context.Context, req *cortexpb.WriteRequest,
	handler WriteHandlerFunc,
) (*cortexpb.WriteResponse, error) {
	return handler(ctx, req)
}

// tracks metric names for which corresponding metadata must be replicated,
// and to which tenants.
type metadataTracker struct {
	tenantTrackedMetrics map[string]map[string]struct{}
}

func newMetadataTracker() *metadataTracker {
	return &metadataTracker{
		tenantTrackedMetrics: make(map[string]map[string]struct{}),
	}
}

func (mt *metadataTracker) replicate(metricMetadata []*cortexpb.MetricMetadata, replicateFn func(tenantId string, metricMetadata []*cortexpb.MetricMetadata) error) []error {
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
	if tenants, ok := mt.tenantTrackedMetrics[metricName]; ok {
		tenants[tenantId] = struct{}{}
	} else {
		mt.tenantTrackedMetrics[metricName] = map[string]struct{}{tenantId: {}}
	}
}
