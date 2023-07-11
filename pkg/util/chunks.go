package util

import (
	"fmt"

	"github.com/prometheus/prometheus/prompb"
	"github.com/samber/lo"
)

type WriteLimit struct {
	// GrpcMaxBytes is the maximum siz of a message that can be sent over the grpc connection.
	GrpcMaxBytes int

	// CortexIngestionRateLimit is the maximum number of samples that can be sent to cortex per second.
	CortexIngestionRateLimit int
}

func DefaultWriteLimit() WriteLimit {
	return WriteLimit{
		GrpcMaxBytes:             4194304,
		CortexIngestionRateLimit: 25000,
	}
}

func splitNChunks[T any](a []T, n int) ([][]T, error) {
	if n == 0 {
		return nil, fmt.Errorf("n cannot be 0")
	}

	var chunks [][]T
	chunkSize := len(a) / n

	if chunkSize == 0 {
		chunkSize = 1
	}

	for start, end := 0, chunkSize; start < len(a); start, end = end, end+chunkSize {
		if end > len(a) {
			end = len(a)
		}
		chunks = append(chunks, a[start:end])
	}

	if len(chunks) < n {
		return nil, fmt.Errorf("slice not large enough to get %d chunks", n)
	}

	return chunks, nil
}

func splitNTimeseriesChunks(ts *prompb.TimeSeries, n int) ([]prompb.TimeSeries, error) {
	sampleChunks, err := splitNChunks(ts.Samples, n)
	if err != nil {
		return nil, err
	}

	return lo.Map(sampleChunks, func(samples []prompb.Sample, _ int) prompb.TimeSeries {
		return prompb.TimeSeries{
			Labels:     ts.Labels,
			Samples:    samples,
			Exemplars:  ts.Exemplars,
			Histograms: ts.Histograms,
		}
	}), nil
}

// splitWriteRequestChunks splits a write request into 2 requests in an attempt to
// lower the amount of data sent in a single request, and return whether or
// not the request could be split. Requests metadata and labels are mever
// modified. We split across multiple fields in the request:
//  1. split containing timeseries into 2 requests until there is only 1 timeseries per request
//  2. split timeseries Samples into 2 requests until there is only 1 sample per request (Exemplars and Histograms are not split)
func splitWriteRequestChunks(request *prompb.WriteRequest, n int) ([]*prompb.WriteRequest, error) {
	switch len(request.Timeseries) {
	case 0:
		return nil, fmt.Errorf("nothing to split in request")
	case 1:
		chunks, err := splitNTimeseriesChunks(&request.Timeseries[0], n)
		if err != nil {
			return nil, fmt.Errorf("could not split request timeseries: %w", err)
		}

		return lo.Map(chunks, func(ts prompb.TimeSeries, _ int) *prompb.WriteRequest {
			return &prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{ts},
				Metadata:   request.Metadata,
			}
		}), nil
	default:
		chunks, err := splitNChunks(request.Timeseries, n)
		if err != nil {
			return nil, fmt.Errorf("could not split request timeseries: %w", err)
		}

		return lo.Map(chunks, func(timeseries []prompb.TimeSeries, _ int) *prompb.WriteRequest {
			return &prompb.WriteRequest{
				Timeseries: timeseries,
				Metadata:   request.Metadata,
			}
		}), nil
	}
}

func sampleCount(request *prompb.WriteRequest) int {
	return lo.Reduce(request.Timeseries, func(count int, ts prompb.TimeSeries, _ int) int {
		return len(ts.Samples) + count
	}, 0)
}

func SplitChunksWithLimit(request *prompb.WriteRequest, limit WriteLimit) ([]*prompb.WriteRequest, error) {
	bytes, err := request.Marshal()
	if err != nil {
		return nil, fmt.Errorf("could not check for ")
	}

	if len(bytes) <= limit.GrpcMaxBytes && sampleCount(request) <= limit.CortexIngestionRateLimit {
		return []*prompb.WriteRequest{request}, nil
	}

	requests, err := splitWriteRequestChunks(request, 2)
	if err != nil {
		return nil, err
	}

	out := make([][]*prompb.WriteRequest, 0, len(requests))
	for _, r := range requests {
		split, err := SplitChunksWithLimit(r, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, split)
	}

	return lo.Flatten(out), nil
}
