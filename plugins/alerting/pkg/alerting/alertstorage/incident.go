package alertstorage

import "google.golang.org/protobuf/types/known/timestamppb"

type State struct {
	Healthy   bool                   `json:"healthy"`
	Firing    bool                   `json:"firing"`
	Timestamp *timestamppb.Timestamp `json:"timestamp"`
}

func (s *State) IsEquivalent(other *State) bool {
	return s.Healthy == other.Healthy && s.Firing == other.Firing
}

type Interval struct {
	Start *timestamppb.Timestamp `json:"start"`
	End   *timestamppb.Timestamp `json:"end"`
}

type IncidentIntervals struct {
	Values []Interval `json:"values"`
}

func NewIncidentIntervals() *IncidentIntervals {
	return &IncidentIntervals{
		Values: []Interval{},
	}
}
