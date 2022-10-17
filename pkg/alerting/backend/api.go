package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
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
	p.Labels["conditionId"] = conditionId
}

// WithRuntimeInfo adds the runtime information to the alert.
func (p *PostableAlert) WithRuntimeInfo(key string, value string) {
	if p.Annotations == nil {
		newMap := map[string]string{}
		p.Annotations = &newMap
	}
	(*p.Annotations)[key] = value
}

func (p *PostableAlert) Must() error {
	if p.Labels == nil {
		return fmt.Errorf("missting PostableAlert.Labels")
	}
	if v, ok := p.Labels["alertname"]; !ok || v == "" {
		return fmt.Errorf(`missting PostableAlert.Labels["alertname"]`)
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
	SilenceId string
}

func (d *DeletableSilence) WithSilenceId(silenceId string) {
	d.SilenceId = silenceId
}

func (d *DeletableSilence) Must() error {
	if d.SilenceId == "" {
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

func (p *PostSilencesResponse) GetSilenceId() string {
	if p == nil || p.SilenceID == nil {
		return ""
	}
	return *p.SilenceID
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

func (p *PostableSilence) Must() error {
	if p.Matchers == nil {
		return fmt.Errorf("missing PostableSilence.Matchers")
	}
	if len(p.Matchers) == 0 {
		return fmt.Errorf("missing PostableSilence.Matchers")
	}
	if p.StartsAt.IsZero() {
		return fmt.Errorf("missing PostableSilence.StartsAt")
	}
	if p.EndsAt.IsZero() {
		return fmt.Errorf("missing PostableSilence.EndsAt")
	}
	return nil
}

func GetAlerts(ctx context.Context, endpoint string) (*http.Request, *http.Response, error) {
	api := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/alerts",
		Verb:     "GET",
	}).WithAPIV2()
	req, err := http.NewRequestWithContext(ctx, api.Verb, api.ConstructHTTP(), nil)
	if err != nil {
		return req, nil, err
	}
	req.Header.Add("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return req, nil, err
	}
	return req, resp, nil
}

// stop/panic on 400
// retry on 404/429 & 500
// returns nil when we indicate nothing more should be done
func OnRetryResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted:
		// do something
		return resp, nil
	case http.StatusBadRequest:
		// panic?
		panic(fmt.Sprintf("%v", req))
	case http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError:
		return http.DefaultClient.Do(req)
	}
	panic(fmt.Sprintf("%v", req))
}

// IsRateLimited assumes the http response status code is checked and validated
func IsRateLimited(conditionId string, resp *http.Response, lg *zap.SugaredLogger) (bool, error) {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			//FIXME
		}
	}(resp.Body)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	statusArr := gjson.Get(string(body), "#.status")
	labelsArr := gjson.Get(string(body), "#.labels")

	if !statusArr.Exists() || !labelsArr.Exists() {
		return false, nil // indicates an empty response (either nil or empty)
	}
	for index, alert := range statusArr.Array() {
		conditionIdPath := labelsArr.Array()[index].Get(shared.BackendConditionIdLabel)
		if !conditionIdPath.Exists() {
			lg.Warnf("missing condition id label '%s' in alert", shared.BackendConditionIdLabel)
			continue
		}
		curConditionId := conditionIdPath.String()
		if curConditionId == conditionId {
			alertState := alert.Get("state").String()
			switch alertState {
			case models.AlertStatusStateActive:
				return true, nil
			case models.AlertStatusStateSuppressed:
				return true, nil
			case models.AlertStatusStateUnprocessed:
				return false, nil
			default:
				return false, nil
			}
		}
	}
	// if not found in AM alerts treat it as unfired => not rate limited
	return false, nil
}

func PostAlert(ctx context.Context, endpoint string, alerts []*PostableAlert) (*http.Request, *http.Response, error) {
	for _, alert := range alerts {
		if err := alert.Must(); err != nil {
			panic(err)
		}
	}
	hclient := &http.Client{}
	reqUrl := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/alerts",
		Verb:     POST,
	}).WithAPIV2().ConstructHTTP()
	b, err := json.Marshal(alerts)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, POST, reqUrl, bytes.NewBuffer(b))
	if err != nil {
		return req, nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	resp, err := hclient.Do(req)
	if err != nil {
		return req, nil, err
	}
	return req, resp, nil
}

func PostSilence(ctx context.Context, endpoint string, silence *PostableSilence) (*http.Response, error) {
	if err := silence.Must(); err != nil {
		panic(err)
	}
	hclient := &http.Client{}
	reqUrl := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/silences",
		Verb:     POST,
	}).WithAPIV2().ConstructHTTP()
	b, err := json.Marshal(silence)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, POST, reqUrl, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	resp, err := hclient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func DeleteSilence(ctx context.Context, endpoint string, silence *DeletableSilence) (*http.Response, error) {
	if err := silence.Must(); err != nil {
		return nil, shared.WithInternalServerErrorf("%s", err)
	}
	hclient := &http.Client{}
	reqUrl := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/silences/" + silence.SilenceId,
		Verb:     DELETE,
	}).WithAPIV2().ConstructHTTP()
	req, err := http.NewRequestWithContext(ctx, DELETE, reqUrl, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hclient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
