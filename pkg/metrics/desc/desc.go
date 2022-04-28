package desc

import (
	"reflect"
	sync "sync"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rancher/opni/pkg/util"
	"golang.org/x/exp/slices"
)

//   Prometheus Desc Clone Memory Model
//
//                            Clone
//                     ┌─────────────────┐
//                     │ state           │
//   Prometheus Desc   │ sizeCache       │
// ┌─────────────────┐ │ unknownFields   │
// │          fqName │ │ FQName          │
// │            help │ │ Help            │
// │ constLabelPairs │ │ ConstLabelPairs │
// │  variableLabels │ │ VariableLabels  │
// │              id │ │ ID              │
// │         dimHash │ │ DimHash         │
// │             err │ │ padding         │
// └─────────────────┘ └─────────────────┘
//

var (
	contentOffset uintptr
	contentSize   uintptr
	once          sync.Once
)

// preliminary type-checking
func init() {
	msg := "the definition of prometheus.Desc has changed; update pkg/metrics/desc"
	protoType := reflect.TypeOf(Desc{})
	promType := reflect.TypeOf(prometheus.Desc{})

	// Find the offset of the first exported field in the proto message.
	firstExportedFieldIndex := -1
	for i := 0; i < protoType.NumField(); i++ {
		field := protoType.Field(i)
		if protoType.Field(i).IsExported() {
			contentSize += field.Type.Size()
			once.Do(func() {
				// save the offset for later use
				contentOffset = field.Offset
				firstExportedFieldIndex = i
			})
		}
	}
	// Make sure the alignments match
	wrongAlign := protoType.Align() != promType.Align()
	// make sure the content sizes match
	wrongContentSize := contentSize != promType.Size()
	// prometheus.Desc has an error field at the end which we ignore
	missingErr := promType.Field(promType.NumField()-1).Type != reflect.TypeOf((*error)(nil)).Elem()
	// +1 to account for the two 8-byte padding fields to match error (16 bytes)
	wrongFieldCount := protoType.NumField()-firstExportedFieldIndex != promType.NumField()+1

	if wrongAlign || wrongContentSize || missingErr || wrongFieldCount {
		panic(msg)
	}
	for i := 0; i < promType.NumField()-1; i++ {
		if protoType.Field(i+firstExportedFieldIndex).Type != promType.Field(i).Type {
			panic(msg)
		}
	}
}

// ToPrometheusDescUnsafe returns a desc.Desc reinterpreted as a prometheus.Desc.
// It is *not* safe to use the original object after calling this function.
func (d *Desc) ToPrometheusDescUnsafe() *prometheus.Desc {
	return (*prometheus.Desc)(unsafe.Add(unsafe.Pointer(d), contentOffset))
}

// ToPrometheusDesc converts a deep-copy of a desc.Desc to a prometheus.Desc.
// It is safe to use the original object after calling this function.
func (d *Desc) ToPrometheusDesc() *prometheus.Desc {
	cloned := &Desc{
		FQName:          d.FQName,
		Help:            d.Help,
		ConstLabelPairs: CloneLabelPairs(d.ConstLabelPairs),
		VariableLabels:  slices.Clone(d.VariableLabels),
		ID:              d.ID,
		DimHash:         d.DimHash,
	}
	return cloned.ToPrometheusDescUnsafe()
}

// FromPrometheusDesc returns a prometheus.Desc reinterpreted as a desc.Desc.
// This function performs a shallow-copy, so it may not be safe to use the
// original object afterwards.
func FromPrometheusDesc(desc *prometheus.Desc) *Desc {
	d := &Desc{}
	*(*prometheus.Desc)(unsafe.Add(unsafe.Pointer(d), contentOffset)) = *desc
	return d
}

func CloneLabelPairs(pairs []*dto.LabelPair) []*dto.LabelPair {
	res := make([]*dto.LabelPair, len(pairs))
	for i, p := range pairs {
		res[i] = &dto.LabelPair{
			Name:  util.Pointer(p.GetName()),
			Value: util.Pointer(p.GetValue()),
		}
	}
	return res
}
