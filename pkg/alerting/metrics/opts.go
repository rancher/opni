/*
 Prometheus metric options for building alerting rules
*/

package metrics

import (
	"fmt"
	"reflect"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
)

const labelTag = "label" // label also requires a metric tag
const jobExtractorTag = "jobExtractor"
const metricTag = "metric"
const rangeTag = "range"

// dummy interface to mark structs as metric options
type MetricOpts interface {
	MetricOptions()
}

// FetchAlertConditionValues :
//
// Iterates over the field values of the MetricOpts struct
// - Fetches labels and annotations from the field tags
// - Returns a map of options to their list of values
func FetchAlertConditionValues(m *MetricOpts, client *cortexadmin.CortexAdminClient) (interface{}, error) {
	values := make(map[string][]string)
	t := reflect.TypeOf(m)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		labelTag := field.Tag.Get(labelTag)
		metricTag := field.Tag.Get(metricTag)
		jobExtractorTag := field.Tag.Get(jobExtractorTag)
		if labelTag != "" {
			if metricTag == "" {
				panic(fmt.Sprintf("invalid struct declaration for : %s, missing metric tag %s on field", t.Kind(), field.Type.String()))
			}
			// fetch label values of on metric
		}

		if field.Tag.Get(jobExtractorTag) != "" {
			// get all metric names matching regex expr
		}

		if field.Tag.Get(rangeTag) != "" {
			// translate rage to an array
		}

		// comparison operator choices
		if reflect.TypeOf(field) == reflect.TypeOf(ComparisonOperator{}) {

		}

		// list clusters on which the metrics are defined
		if reflect.TypeOf(field) == reflect.TypeOf(corev1.Cluster{}) {

		}

		// list multiples of time.Duration
		if reflect.TypeOf(field) == reflect.TypeOf(time.Duration(0)) {

		}
	}
	return values, nil
}

type ComparisonOperator struct {
	value string
}

func (c *ComparisonOperator) String() string {
	return c.value
}

func NewComparisonOperator(value string) (*ComparisonOperator, error) {
	switch value {
	case ">", "<", "=", "!=", ">=", "<=":
		return &ComparisonOperator{value: value}, nil
	default:
		return nil, validation.Errorf("Invalid comparison operator: %s", value)
	}
}
