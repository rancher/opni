package cortex

import (
	"container/heap"
	"math"
	"time"

	"github.com/prometheus/common/model"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func promValueToUnix(v *model.SamplePair) time.Time {
	// note prometheus unmarshals to float64 samples, but the timestamp values
	// are int64 with unix nano seconds, so math.Round is predictable
	return time.Unix(int64(math.Round(float64(v.Value))), 0)
}

func between(start, end, ts time.Time) bool {
	return ts.After(start) && ts.Before(end)
}

// `ALERTS` can have several root causes for causing the active alerting rule to fire;
// prometheus store each root cause as a separate synthetic timeseries.
// This function reduces the matrix of synthetic timeseries into a single array of sorted active windows
// !! Assumes the input matrix is non-nil
func ReducePrometheusMatrix(matrix *model.Matrix) []*alertingv1.ActiveWindow {
	timeline := make([]*alertingv1.ActiveWindow, 0)
	processingHeap := &MatrixHeap{}

	heap.Init(processingHeap)
	for i, metric := range *matrix {
		if len(metric.Values) > 0 {
			heap.Push(processingHeap, MatrixRef{A: i, B: 0, C: &metric.Values[0]})
		}
	}
	for processingHeap.Len() > 0 {
		ref := heap.Pop(processingHeap).(MatrixRef)

		// increment next value of metric.Values to process
		if ref.B+1 < len((*matrix)[ref.A].Values) {
			heap.Push(processingHeap, MatrixRef{A: ref.A, B: ref.B + 1, C: &(*matrix)[ref.A].Values[ref.B+1]})
		}

		// Incidents are uniquely identified in the `ALERTS_FOR_STATE` metric by their value which is
		// a "fingerprint" timestamp, note that this fingerprint timestamp changes if the labels on
		// the alerting rule change, and thus will be considered separate incidents

		// We can consider this fingerprint timestamp as the source time for the alarm

		fingerprintStr := ref.C
		fingerprintTs, curTs := promValueToUnix(ref.C), ref.C.Timestamp.Time()
		if len(timeline) == 0 {
			timeline = append(timeline, &alertingv1.ActiveWindow{
				Start:        timestamppb.New(fingerprintTs),
				End:          timestamppb.New(curTs),
				Type:         alertingv1.TimelineType_Timeline_Alerting,
				Fingerprints: []string{fingerprintStr.Value.String()},
			})
			continue
		}

		last := timeline[len(timeline)-1]
		startTs, endTs := last.Start.AsTime(), last.End.AsTime()

		if startTs.Equal(fingerprintTs) {
			last.End = timestamppb.New(curTs)
		} else if startTs.After(fingerprintTs) || between(startTs, endTs, fingerprintTs) {
			if curTs.After(last.End.AsTime().Add(time.Second * 90)) {
				// last.End = curTs
				timeline = append(timeline, &alertingv1.ActiveWindow{
					Start:        timestamppb.New(last.End.AsTime().Add(time.Minute)),
					End:          timestamppb.New(curTs),
					Type:         alertingv1.TimelineType_Timeline_Alerting,
					Fingerprints: []string{fingerprintStr.Value.String()},
				})
			} else {
				last.End = timestamppb.New(curTs)
				last.Fingerprints = append(last.Fingerprints, fingerprintStr.Value.String())
			}
		} else {
			timeline = append(timeline, &alertingv1.ActiveWindow{
				Start:        timestamppb.New(fingerprintTs),
				End:          timestamppb.New(curTs),
				Type:         alertingv1.TimelineType_Timeline_Alerting,
				Fingerprints: []string{fingerprintStr.Value.String()},
			})
		}
	}
	for _, t := range timeline {
		t.Fingerprints = lo.Uniq(t.Fingerprints)
	}
	return timeline
}

// MatrixRef
// A : Refers to the index of a specific metric in the matrix
// B : Refers to the position in the metric.Values array to process
type MatrixRef lo.Tuple3[int, int, *model.SamplePair]

// Implements the container/heap.Heap interface
type MatrixHeap []MatrixRef

func (h MatrixHeap) Len() int { return len(h) }
func (h MatrixHeap) Less(i, j int) bool {
	return h[i].C.Timestamp.Before(h[j].C.Timestamp)
}
func (h MatrixHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *MatrixHeap) Push(x any) {
	*h = append(*h, x.(MatrixRef))
}

func (h *MatrixHeap) Pop() any {
	old := *h
	n := len(*h)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
