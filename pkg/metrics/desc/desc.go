package desc

import (
	"reflect"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rancher/opni-monitoring/pkg/util"
	"golang.org/x/exp/slices"
)

var descpbContentOffset uintptr

// preliminary type-checking
func init() {
	msg := "the definition of prometheus.Desc has changed; update pkg/metrics/descpb"
	protoType := reflect.TypeOf(Desc{})
	promType := reflect.TypeOf(prometheus.Desc{})
	if protoType.Align() != promType.Align() {
		panic(msg)
	}

	// Find the offset of the first exported field in the proto message.
	firstExportedFieldIndex := -1
	for i := 0; i < protoType.NumField(); i++ {
		field := protoType.Field(i)
		if protoType.Field(i).IsExported() {
			// save the offset for later use
			descpbContentOffset = field.Offset
			firstExportedFieldIndex = i
			break
		}
	}

	// prometheus.Desc has an error field at the end which we ignore
	if promType.Field(promType.NumField()-1).Type != reflect.TypeOf((*error)(nil)).Elem() {
		panic(msg)
	}

	// +1 to account for the two 8-byte padding fields to match error (16 bytes)
	if protoType.NumField()-firstExportedFieldIndex != promType.NumField()+1 {
		panic(msg)
	}
	for i := 0; i < promType.NumField()-1; i++ {
		if protoType.Field(i+firstExportedFieldIndex).Type != promType.Field(i).Type {
			panic(msg)
		}
	}
}

// ToPrometheusDescUnsafe returns a descpb.Desc reinterpreted as a prometheus.Desc.
// It is *not* safe to use the original object after calling this function.
func (d *Desc) ToPrometheusDescUnsafe() *prometheus.Desc {
	return (*prometheus.Desc)(unsafe.Add(unsafe.Pointer(d), descpbContentOffset))
}

// ToPrometheusDesc converts a deep-copy of a descpb.Desc to a prometheus.Desc.
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

// FromPrometheusDesc returns a prometheus.Desc reinterpreted as a descpb.Desc.
// This function performs a shallow-copy, so it may not be safe to use the
// original object afterwards.
func FromPrometheusDesc(desc *prometheus.Desc) *Desc {
	d := &Desc{}
	*(*prometheus.Desc)(unsafe.Add(unsafe.Pointer(d), descpbContentOffset)) = *desc
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
