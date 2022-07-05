package query

import "math"

/*
This module defines matcher functions for finding the best match for the
discovered downstream metrics
*/

func MatchMinLength(metrics []string) string {
	min := math.MaxInt // largest int
	metricId := ""
	for _, m := range metrics {
		if m == "" {
			continue
		}
		if len(m) < min {
			min = len(m)
			metricId = m
		}
	}
	return metricId
}
