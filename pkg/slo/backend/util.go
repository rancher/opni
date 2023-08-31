package backend

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/samber/lo"
)

const (
	// id labels
	SLOUuid    = "slo_opni_id"
	SLOService = "slo_opni_service"
	SLOName    = "slo_opni_name"
)

type Service string
type Metric string

type LabelPairs []LabelPair
type LabelPair struct {
	Key  string
	Vals []string
}

type IdentificationLabels map[string]string

func (i IdentificationLabels) Validate() error {
	if _, ok := i[SLOUuid]; !ok {
		return fmt.Errorf("slo uuid label '%s' is required", SLOUuid)
	}
	if _, ok := i[SLOService]; !ok {
		return fmt.Errorf("slo service label '%s' is required", SLOService)
	}
	if _, ok := i[SLOName]; !ok {
		return fmt.Errorf("slo name label '%s' is required", SLOName)
	}
	return nil
}

func (i IdentificationLabels) ToLabels() LabelPairs {
	lbs := lo.MapToSlice(i, func(k string, v string) LabelPair {
		return LabelPair{
			Key:  k,
			Vals: []string{v},
		}
	})

	slices.SortFunc(lbs, func(a, b LabelPair) int {
		if a.Key < b.Key {
			return -1
		} else if a.Key > b.Key {
			return 1
		}
		return 0
	})
	return lbs
}

func (i IdentificationLabels) JoinOnPrometheus() string {
	keys := lo.Keys(i)
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func (l LabelPairs) ConstructPrometheus() string { // kinda hacky & technically unoptimized but works
	if len(l) == 0 {
		return ""
	}
	filterStrings := []string{}
	for _, labelPair := range l {
		if labelPair.Key == "" || len(labelPair.Vals) == 0 {
			continue
		}
		filterString := fmt.Sprintf("%s=~\"%s\"", labelPair.Key, strings.Join(labelPair.Vals, "|"))
		filterStrings = append(filterStrings, filterString)
	}
	return strings.Join(filterStrings, ",")
}

func LeftJoinSlice[T comparable](arr1, arr2 []T) []T {
	result := make([]T, len(arr1))
	cache := map[T]struct{}{}
	for i, v := range arr1 {
		cache[v] = struct{}{}
		result[i] = v
	}
	for _, v := range arr2 {
		if _, ok := cache[v]; !ok {
			result = append(result, v)
		}
	}
	return result
}
