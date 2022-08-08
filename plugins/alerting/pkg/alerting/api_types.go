package alerting

import (
	"fmt"
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

func (p *PostableAlert) Validate() error {
	if p.Labels == nil {
		return fmt.Errorf("Missting PostableAlert.Labels")
	}
	if v, ok := p.Labels["alertname"]; !ok || v == "" {
		return fmt.Errorf(`Missting PostableAlert.Labels["alertname"]`)
	}
	return nil
}

type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual *bool  `json:"isEqual,omitempty"`
}

// DeletableSilence fills in api path `"/silence/{silenceID}"`
type DeletableSilence struct {
	silenceId string
}

func (d *DeletableSilence) WithSilenceId(silenceId string) {
	d.silenceId = silenceId
}

func (d *DeletableSilence) Validate() error {
	if d.silenceId == "" {
		return fmt.Errorf("missing silenceId")
	}
	return nil
}

// PostableSilence struct for PostableSilence
type PostableSilence struct {
	Id        *string   `json:"id,omitempty"`
	Matchers  []Matcher `json:"matchers"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	CreatedBy string    `json:"createdBy"`
	Comment   string    `json:"comment"`
}

type PostSilencesResponse struct {
	SilenceID *string `json:"silenceID,omitempty"`
}

// WithCondition In our basic model each receiver is uniquely
// identified by its name, which in AlertManager can be routed
// to by the label `alertname`.
func (p *PostableSilence) WithCondition(conditionId string) {
	if p.Matchers == nil {
		p.Matchers = make([]Matcher, 0)
	}
	p.Matchers = append(p.Matchers, Matcher{Name: conditionId})
}

func (p *PostableSilence) WithDuration(dur time.Duration) {
	start := time.Now()
	end := start.Add(dur)
	p.StartsAt = start
	p.EndsAt = end
}

func (p *PostableSilence) WithSilenceId(silenceId string) {
	p.Id = &silenceId
}

func (p *PostableSilence) Validate() error {
	if p.Matchers == nil {
		return fmt.Errorf("Missting PostableSilence.Matchers")
	}
	if len(p.Matchers) == 0 {
		return fmt.Errorf("Missting PostableSilence.Matchers")
	}
	if p.StartsAt.IsZero() {
		return fmt.Errorf("Missting PostableSilence.StartsAt")
	}
	if p.EndsAt.IsZero() {
		return fmt.Errorf("Missting PostableSilence.EndsAt")
	}
	return nil
}
