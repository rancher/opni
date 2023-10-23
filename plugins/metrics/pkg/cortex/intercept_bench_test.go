package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"
)

var sampledata []byte

func init() {
	writeRequest := &cortexpb.WriteRequest{
		Timeseries: cortexpb.PreallocTimeseriesSliceFromPool(),
	}

	now := time.Now()

	tenants := []string{"", "tenant1", "tenant2", "tenant3", "tenant4"}
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("test_metric_%d", i)
		for _, tenant := range tenants {
			for _, sampleValue := range []string{"a", "b", "c", "d"} {
				ts := cortexpb.TimeseriesFromPool()
				ts.Labels = []cortexpb.LabelAdapter{
					{
						Name:  "__name__",
						Value: name,
					},
					{
						Name:  "instance",
						Value: fmt.Sprintf("localhost"),
					},
					{
						Name:  "job",
						Value: "intercept_test",
					},
					{
						Name:  "sample_label",
						Value: sampleValue,
					},
				}
				if tenant != "" {
					ts.Labels = append(ts.Labels, cortexpb.LabelAdapter{
						Name:  metrics.LabelImpersonateAs,
						Value: tenant,
					})
				}
				ts.Samples = []cortexpb.Sample{
					{
						Value:       rand.Float64(),
						TimestampMs: now.UnixMilli(),
					},
				}
				writeRequest.Timeseries = append(writeRequest.Timeseries, cortexpb.PreallocTimeseries{ts})
			}
		}
	}

	sampledata, _ = json.Marshal(writeRequest)
	os.WriteFile("testdata/gen.json", sampledata, 0644)
	cortexpb.ReuseSlice(writeRequest.Timeseries)
}

func Benchmark_federatingInterceptor_Intercept(b *testing.B) {
	interceptor := NewFederatingInterceptor(InterceptorConfig{
		IdLabelName: metrics.LabelImpersonateAs,
	})

	b.ReportAllocs()
	// set up write requests
	inputs := make([]*cortexpb.WriteRequest, 0, b.N)
	for i := 0; i < b.N; i++ {
		req := cortexpb.WriteRequest{}
		err := json.Unmarshal(sampledata, &req)
		if err != nil {
			panic(err)
		}
		inputs = append(inputs, &req)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wr := inputs[i]
		ctx, _ := user.InjectIntoGRPCRequest(user.InjectOrgID(context.Background(), "test"))
		interceptor.Intercept(ctx, wr, func(ctx context.Context, wr *cortexpb.WriteRequest, co ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
			return nil, nil
		})
	}
}

func Test_federatingInterceptor_Intercept(t *testing.T) {
	interceptor := NewFederatingInterceptor(InterceptorConfig{
		IdLabelName: metrics.LabelImpersonateAs,
	})
	wr := cortexpb.WriteRequest{}
	err := json.Unmarshal(sampledata, &wr)
	if err != nil {
		panic(err)
	}
	ctx, _ := user.InjectIntoGRPCRequest(user.InjectOrgID(context.Background(), "test"))
	_, err = interceptor.Intercept(ctx, &wr, func(ctx context.Context, wr *cortexpb.WriteRequest, co ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
		return nil, nil
	})
	if err != nil {
		t.Error("error", logger.Err(err))
		os.Exit(1)
	}
}

type testCase struct {
	name string
	arr  []*element
	want []*Partition
}
type element struct {
	Id   string
	Data int
}

func TestPartitionById(t *testing.T) {
	tests := []testCase{
		{
			name: "Empty array",
			arr:  []*element{},
			want: nil,
		},
		{
			name: "Array with one ID only",
			arr: []*element{
				{"id0", 1},
				{"id0", 2},
				{"id0", 3},
			},
			want: []*Partition{
				{"id0", 0, 3},
			},
		},
		{
			name: "Array with multiple IDs",
			arr: []*element{
				{"id0", 1},
				{"id1", 2},
				{"id0", 3},
				{"id2", 4},
				{"id1", 5},
				{"id2", 6},
			},
			want: []*Partition{
				{"id0", 0, 2},
				{"id1", 2, 4},
				{"id2", 4, 6},
			},
		},
	}
	for i := 0; i < 100; i++ {
		tests = append(tests, makeTestCase(makeRandomArray()))
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := 0
			PartitionById(tt.arr,
				func(idx int) string {
					return tt.arr[idx].Id
				},
				func(id string, start, end int) {
					if id != tt.want[i].Id || start != tt.want[i].Start || end != tt.want[i].End {
						t.Errorf("at index %d: got [%s, %d:%d], want %v", i, id, start, end, tt.want[i])
					}
					i++
				})
		})
	}
}

func BenchmarkPartitionById(b *testing.B) {
	b.Run("Random Elements", func(b *testing.B) {
		tests := []testCase{}
		for i := 0; i < b.N; i++ {
			tests = append(tests, makeTestCase(makeRandomArray()))
		}

		b.ResetTimer()

		for _, t := range tests {
			PartitionById(t.arr, func(idx int) string {
				return t.arr[idx].Id
			}, func(id string, start, end int) {})
		}
	})

	b.Run("Grouped Elements", func(b *testing.B) {
		tests := []testCase{}
		for i := 0; i < b.N; i++ {
			tests = append(tests, makeTestCase(makeGroupedArray()))
		}

		b.ResetTimer()

		for _, t := range tests {
			PartitionById(t.arr, func(idx int) string {
				return t.arr[idx].Id
			}, func(id string, start, end int) {})
		}
	})
}

func makeRandomArray() []*element {
	arr := make([]*element, 1000)
	for i := range arr {
		arr[i] = &element{fmt.Sprintf("id%d", rand.Intn(10)), i}
	}
	return arr
}

func makeGroupedArray() []*element {
	numGroups := 100
	groupIds := make([]int, numGroups)
	for i := range groupIds {
		groupIds[i] = rand.Intn(10)
	}

	partitions := make([]int, numGroups) // indexes
	for i := 0; i < numGroups-1; i++ {
		p := rand.Intn(1000)
		for _, existing := range partitions {
			if p == existing {
				p = rand.Intn(1000)
			} else {
				break
			}
		}
		partitions[i] = p
	}
	partitions[numGroups-1] = 1000

	arr := make([]*element, 0, 1000)
	arrIdx := 0
	for i := 0; i < numGroups; i++ {
		value := groupIds[i]
		end := partitions[i]

		for arrIdx < end {
			arr = append(arr, &element{fmt.Sprintf("id%d", value), arrIdx})
			arrIdx++
		}
	}

	return arr
}

func makeTestCase(arr []*element) testCase {
	// generate random array

	orderOfAppearance := []string{arr[0].Id}
	counts := map[string]int{arr[0].Id: 1}
	for i := len(arr) - 1; i > 0; i-- {
		e := arr[i]
		if _, ok := counts[e.Id]; !ok {
			orderOfAppearance = append(orderOfAppearance, e.Id)
		}
		counts[e.Id]++
	}

	partitionOrder := make([]string, 0, len(orderOfAppearance))
	partitionOrder = append(partitionOrder, orderOfAppearance[0])
	for i := len(orderOfAppearance) - 1; i > 0; i-- {
		partitionOrder = append(partitionOrder, orderOfAppearance[i])
	}

	// generate expected partitions
	want := []*Partition{}
	pos := 0
	for _, id := range partitionOrder {
		want = append(want, &Partition{id, pos, pos + counts[id]})
		pos += counts[id]
	}
	return testCase{
		name: "Random array",
		arr:  arr,
		want: want,
	}
}
