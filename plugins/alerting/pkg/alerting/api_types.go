package alerting

import (
	"time"
)

// PostableAlert : corresponds to the data AlertManager API
// needs to trigger alerts
type PostableAlert struct {
	StartsAt     *time.Time         `json:"startsAt,omitempty"`
	EndsAt       *time.Time         `json:"endsAt,omitempty"`
	Annotations  *map[string]string `json:"annotations,omitempty"`
	Labels       map[string]string  `json:"labels"`
	GeneratorURL *string            `json:"generatorURL,omitempty"`
}

// WithCondition In our basic model each receiver is uniquely
// identified by its name, which in AlertManager can be routed
// to by the label `alertname`.
func (p *PostableAlert) WithCondition(conditionId string) {
	if p.Labels == nil {
		p.Labels = make(map[string]string)
	}
	p.Labels["alertname"] = conditionId
}

// WithRuntimeInfo adds the runtime information to the alert.
//
func (p *PostableAlert) WithRuntimeInfo(key string, value string) {
	if p.Annotations == nil {
		newMap := map[string]string{}
		p.Annotations = &newMap
	}
	(*p.Annotations)[key] = value
}
