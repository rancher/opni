package routing

import (
	"fmt"

	cfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/shared"
	"gopkg.in/yaml.v3"
)

type OpniRoutingMetadata struct {
	EndpointType string `yaml:"endpointType" json:"endpointType"`
	Position     *int   `yaml:"position" json:"position"`
}

// conditionId --> endpointId matching the current configuration of Route
type OpniInternalRouting struct {
	Content map[string]map[string]*OpniRoutingMetadata `yaml:"inline,omitempty" json:"inline,omitempty"`
}

func (o *OpniInternalRouting) DeepCopy() (*OpniInternalRouting, error) {
	data, err := yaml.Marshal(o)
	if err != nil {
		return nil, err
	}
	newOpniInternalRouting := &OpniInternalRouting{}
	err = yaml.Unmarshal(data, newOpniInternalRouting)
	if err != nil {
		return nil, err
	}
	return newOpniInternalRouting, nil
}

// RoutingTree
//
// When creating a new receiver (routingNode), our assumption is that name == conditionId
// When creating a new route to this routinegNode the assumption is that its receiver name
// matches the receiver and the conditionId the route is associated with.
//
// Marshals/Unmarshals to github.com/prometheus/alertmanager/config/config.go's Config struct,
// but we don't use this struct directly due to :
// - package mismatched versions with prometheus/common
// - override marshalling of secrets that prevents us from putting them into a secret
//
// # RoutingTree must always marshal to AlertManager's config, otherwise backend will reject it
//
// We also assume that EndpointImplementation will have the same config for each receiver for the same
// condition ID
type RoutingTree struct {
	Global       *GlobalConfig      `yaml:"global,omitempty" json:"global,omitempty"`
	Route        *cfg.Route         `yaml:"route,omitempty" json:"route,omitempty"`
	InhibitRules []*cfg.InhibitRule `yaml:"inhibit_rules,omitempty" json:"inhibit_rules,omitempty"`
	Receivers    []*Receiver        `yaml:"receivers,omitempty" json:"receivers,omitempty"`
	Templates    []string           `yaml:"templates" json:"templates"`
}

func areDurationsEqual(m1, m2 *model.Duration) (equal bool, reason string) {
	if m1 == nil && m2 == nil {
		return true, ""
	} else if m1 == nil && m2 != nil || m1 != nil && m2 == nil {
		return false, "one of the durations is nil"
	}
	if m1.String() != m2.String() {
		return false, fmt.Sprintf("durations mismatch %s <-> %s", m1.String(), m2.String())
	}
	return true, ""

}

func slackConfigsAreEqual(s1, s2 *SlackConfig) (equal bool, reason string) {
	if s1.Channel != s2.Channel {
		return false, fmt.Sprintf("channel mismatch  %s <-> %s ", s1.Channel, s2.Channel)
	}
	if s1.APIURL != s2.APIURL {
		return false, fmt.Sprintf("api url mismatch  %s <-> %s ", s1.APIURL, s2.APIURL)
	}
	return true, ""
}

func emailConfigsAreEqual(e1, e2 *EmailConfig) (equal bool, reason string) {
	if e1.To != e2.To {
		return false, fmt.Sprintf("to mismatch %s <-> %s", e1.To, e2.To)
	}
	if e1.From != e2.From {
		return false, fmt.Sprintf("from mismatch %s <-> %s ", e1.From, e2.From)
	}
	if e1.Smarthost != e2.Smarthost {
		return false, fmt.Sprintf("smarthost mismatch %s <-> %s ", e1.Smarthost, e2.Smarthost)
	}
	if e1.AuthUsername != e2.AuthUsername {
		return false, fmt.Sprintf("auth username mismatch %s <-> %s ", e1.AuthUsername, e2.AuthUsername)
	}
	if e1.AuthPassword != e2.AuthPassword {
		return false, fmt.Sprintf("auth password mismatch %s <-> %s ", e1.AuthPassword, e2.AuthPassword)
	}
	if e1.AuthSecret != e2.AuthSecret {
		return false, fmt.Sprintf("auth secret mismatch %s <-> %s ", e1.AuthSecret, e2.AuthSecret)
	}
	if e1.RequireTLS != e2.RequireTLS {
		return false, fmt.Sprintf("require tls mismatch %v <-> %v ", e1.RequireTLS, e2.RequireTLS)
	}
	if e1.HTML != e2.HTML {
		return false, fmt.Sprintf("html mismatch %s <-> %s ", e1.HTML, e2.HTML)
	}
	if e1.Text != e2.Text {
		return false, fmt.Sprintf("text mismatch %s <-> %s ", e1.Text, e2.Text)
	}
	return true, ""
}

func pagerDutyConfigsAreEqual(p1, p2 *PagerdutyConfig) (equal bool, reason string) {
	if p1.RoutingKey != p2.RoutingKey {
		return false, fmt.Sprintf("routing key mismatch %s <-> %s ", p1.RoutingKey, p2.RoutingKey)
	}
	if p1.ServiceKey != p2.ServiceKey {
		return false, fmt.Sprintf("service key mismatch %s <-> %s ", p1.ServiceKey, p2.ServiceKey)
	}
	if p1.URL != p2.URL {
		return false, fmt.Sprintf("url mismatch %s <-> %s ", p1.URL, p2.URL)
	}
	if p1.Client != p2.Client {
		return false, fmt.Sprintf("client mismatch %s <-> %s ", p1.Client, p2.Client)
	}
	if p1.ClientURL != p2.ClientURL {
		return false, fmt.Sprintf("client url mismatch %s <-> %s ", p1.ClientURL, p2.ClientURL)
	}
	if p1.Description != p2.Description {
		return false, fmt.Sprintf("description mismatch %s <-> %s ", p1.Description, p2.Description)
	}
	return true, ""
}

func receiversAreEqual(r1 *Receiver, r2 *Receiver) (equal bool, reason string) {
	if r1.Name != r2.Name { // opni specific indexing
		return false, fmt.Sprintf("receiver name mismatch %s <-> %s ", r1.Name, r2.Name)
	}
	if len(r1.EmailConfigs) != len(r2.EmailConfigs) {
		return false, fmt.Sprintf("email configs are not yet synced: found num old %d <-> num new %d ", len(r1.EmailConfigs), len(r2.EmailConfigs))
	}

	if len(r1.SlackConfigs) != len(r2.SlackConfigs) {
		return false, fmt.Sprintf("slack configs are not yet synced: found num old %d <-> num new %d ", len(r1.SlackConfigs), len(r2.SlackConfigs))
	}
	if len(r1.PagerdutyConfigs) != len(r2.PagerdutyConfigs) {
		return false, fmt.Sprintf("pager duty configs are not yet synced: found num old %d <-> num new %d ", len(r1.PagerdutyConfigs), len(r2.PagerdutyConfigs))
	}
	for idx, emailConfig := range r1.EmailConfigs {
		if equal, reason := emailConfigsAreEqual(emailConfig, r2.EmailConfigs[idx]); !equal {
			return false, fmt.Sprintf("email config mismatch : %s", reason)
		}
	}
	for idx, slackConfig := range r1.SlackConfigs {
		if equal, reason := slackConfigsAreEqual(slackConfig, r2.SlackConfigs[idx]); !equal {
			return false, fmt.Sprintf("slack config mismatch %s", reason)
		}
	}
	for idx, pagerDutyConfig := range r1.PagerdutyConfigs {
		if equal, reason := pagerDutyConfigsAreEqual(pagerDutyConfig, r2.PagerdutyConfigs[idx]); !equal {
			return false, fmt.Sprintf("pager duty config mismatch %s", reason)
		}
	}
	return true, ""
}

func routesAreEqual(r1 *cfg.Route, r2 *cfg.Route) (equal bool, reason string) {
	if len(r1.Routes) != len(r2.Routes) {
		return false, fmt.Sprintf("route length mismatch %d <-> %d ", len(r1.Routes), len(r2.Routes))
	}
	if r1.Continue != r2.Continue {
		return false, fmt.Sprintf("continue mismatch %v <-> %v ", r1.Continue, r2.Continue)
	}
	if equal, reason := areDurationsEqual(r1.RepeatInterval, r2.RepeatInterval); !equal {
		return false, fmt.Sprintf("repeat interval mismatch %s ", reason)
	}
	if equal, reason := areDurationsEqual(r1.GroupInterval, r2.GroupInterval); !equal {
		return false, fmt.Sprintf("group interval mismatch %s", reason)
	}
	if r1.Receiver != r2.Receiver {
		return false, fmt.Sprintf("receiver mismatch %s <-> %s ", r1.Receiver, r2.Receiver)
	}
	if !areMatchersEqual(r1.Matchers, r2.Matchers) {
		return false, "matchers mismatch"
	}
	return true, ""
}

func (r *RoutingTree) indexOpniReceivers() map[string]*Receiver {
	selfReceiverIndex := map[string]*Receiver{}
	for _, receiver := range r.Receivers {
		selfReceiverIndex[receiver.Name] = receiver
	}
	return selfReceiverIndex
}

func (r *RoutingTree) indexOpniRoutes() map[string]*cfg.Route {
	selfRoutingIndex := map[string]*cfg.Route{}
	for _, route := range r.Route.Routes {
		exists := false
		conditionId := ""
		for _, matcher := range route.Matchers {
			if matcher.Name == shared.BackendConditionIdLabel {
				exists = true
				conditionId = matcher.Value
				break
			}
		}
		if !exists {
			continue
		}
		selfRoutingIndex[conditionId] = route
	}
	return selfRoutingIndex
}

func areMatchersEqual(m1, m2 cfg.Matchers) bool {
	if len(m1) != len(m2) {
		return false
	}
	m1Map := map[string]string{} // name --> value
	m2Map := map[string]string{} // name --> value
	for _, matcher := range m1 {
		m1Map[matcher.Name] = matcher.Value
	}
	for _, matcher := range m2 {
		m2Map[matcher.Name] = matcher.Value
	}
	for name, value := range m1Map {
		if _, ok := m2Map[name]; !ok {
			return false
		}
		if m2Map[name] != value {
			return false
		}
	}
	return true
}

// for our purposes we will only treat receivers and routes as opni config equality
func (r *RoutingTree) IsEqual(other *RoutingTree) (equal bool, reason string) {
	selfReceiverIndex := r.indexOpniReceivers()
	otherReceiverIndex := other.indexOpniReceivers()
	for id, r1 := range selfReceiverIndex {
		if r2, ok := otherReceiverIndex[id]; !ok {
			return false, fmt.Sprintf("configurations do not have matching receiver : %s", id)
		} else {
			if equal, reason := receiversAreEqual(r1, r2); !equal {
				return false, fmt.Sprintf("configurations do not have equal receivers '%s' : %s", id, reason)
			}
		}
	}
	selfRoutingIndex := r.indexOpniRoutes()
	otherRoutingIndex := other.indexOpniRoutes()
	for id, r1 := range selfRoutingIndex {
		if r2, ok := otherRoutingIndex[id]; !ok {
			return false, fmt.Sprintf("configurations do not have matching route : %s", id)
		} else {
			if equal, reason := routesAreEqual(r1, r2); !equal {
				return false, fmt.Sprintf("configurations do not have equal route '%s' : %s", id, reason)
			}
		}
	}
	return true, ""
}

func (r *RoutingTree) DeepCopy() (*RoutingTree, error) {
	data, err := yaml.Marshal(r)
	if err != nil {
		return nil, err
	}
	newRoutingTree := &RoutingTree{}
	err = yaml.Unmarshal(data, newRoutingTree)
	if err != nil {
		return nil, err
	}
	return newRoutingTree, nil
}

func (r *RoutingTree) Parse(data string) error {
	return yaml.Unmarshal([]byte(data), r)
}

func NewRoutingTreeFrom(data string) (*RoutingTree, error) {
	c := &RoutingTree{}
	err := c.Parse(data)
	return c, err
}

func (r *RoutingTree) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(r)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func NewDefaultOpniInternalRouting() *OpniInternalRouting {
	return &OpniInternalRouting{}
}

func (o *OpniInternalRouting) Parse(data string) error {
	return yaml.Unmarshal([]byte(data), o)
}

func (o *OpniInternalRouting) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(o)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func NewOpniInternalRoutingFrom(raw string) (*OpniInternalRouting, error) {
	c := &OpniInternalRouting{}
	err := c.Parse(raw)
	return c, err
}

func (o *OpniInternalRouting) Add(
	conditionId string,
	endpointId string,
	metadata OpniRoutingMetadata) error {
	if metadata.EndpointType == "" {
		return fmt.Errorf("endpointTypeRequired")
	}
	if metadata.Position == nil || *metadata.Position < 0 {
		return fmt.Errorf("positionRequired")
	}
	if o.Content == nil {
		o.Content = make(map[string]map[string]*OpniRoutingMetadata)
	}
	if o.Content[conditionId] == nil {
		o.Content[conditionId] = make(map[string]*OpniRoutingMetadata)
	}
	if _, ok := o.Content[conditionId]; !ok {
		o.Content[conditionId][endpointId] = &OpniRoutingMetadata{
			EndpointType: metadata.EndpointType,
			Position:     metadata.Position,
		}
	} else {
		if _, ok := o.Content[conditionId][endpointId]; !ok {
			o.Content[conditionId][endpointId] = &OpniRoutingMetadata{
				EndpointType: metadata.EndpointType,
				Position:     metadata.Position,
			}
		} else {
			return shared.WithFailedPreconditionError("(conditionId, endpointId) key already exists and should not")
		}
	}
	return nil
}

func (o *OpniInternalRouting) GetFromCondition(conditionId string) (map[string]*OpniRoutingMetadata, error) {
	if res, ok := o.Content[conditionId]; !ok {
		return nil, shared.WithNotFoundError("existing condition id that is expected to exist not found in internal routing ids")
	} else {
		return res, nil
	}
}

func (o *OpniInternalRouting) Get(conditionId, endpointId string) (*OpniRoutingMetadata, error) {
	if condition, ok := o.Content[conditionId]; !ok {
		return nil, shared.WithNotFoundError("existing condition id that is expected to exist not found in internal routing ids")
	} else {
		if metadata, ok := condition[endpointId]; !ok {
			return nil, shared.WithNotFoundError("existing condition/endpoint id pair that is expected to exist not found in internal routing ids")
		} else {
			return metadata, nil
		}
	}
}

func (o *OpniInternalRouting) UpdateEndpoint(conditionId, notificationId string, metadata OpniRoutingMetadata) error {
	if metadata.EndpointType == "" {
		return fmt.Errorf("endpointTypeRequired")
	}
	if metadata.Position == nil || *metadata.Position < 0 {
		return fmt.Errorf("positionRequired")
	}
	if _, ok := o.Content[conditionId]; !ok {
		return shared.WithNotFoundError("existing condition id that is expected to exist not found in internal routing ids")
	}
	if _, ok := o.Content[conditionId][notificationId]; !ok {
		return shared.WithNotFoundError("existing condition/endpoint id pair that is expected to exist not found in internal routing ids")
	}
	o.Content[conditionId][notificationId] = &OpniRoutingMetadata{
		EndpointType: metadata.EndpointType,
		Position:     metadata.Position,
	}
	return nil
}

func (o *OpniInternalRouting) RemoveEndpoint(conditionId string, endpointId string) error {
	if _, ok := o.Content[conditionId]; !ok {
		return shared.WithNotFoundError("existing condition id that is expected to exist not found in internal routing ids")
	}
	if _, ok := o.Content[conditionId][endpointId]; !ok {
		return shared.WithNotFoundError("existing condition & endpoint id pair that is expected to exist not found in internal routing ids")
	}
	delete(o.Content[conditionId], endpointId)
	return nil
}

func (o *OpniInternalRouting) RemoveCondition(conditionId string) error {
	if _, ok := o.Content[conditionId]; !ok {
		return shared.WithNotFoundError("existing condition id that is expected to exist not found in internal routing ids")
	} else {
		delete(o.Content, conditionId)
	}
	return nil
}
