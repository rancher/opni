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
		if equal, reason := emailConfig.Equal(r2.EmailConfigs[idx]); !equal {
			return false, fmt.Sprintf("email config mismatch : %s", reason)
		}
	}
	for idx, slackConfig := range r1.SlackConfigs {
		if equal, reason := slackConfig.Equal(r2.SlackConfigs[idx]); !equal {
			return false, fmt.Sprintf("slack config mismatch %s", reason)
		}
	}
	for idx, pagerDutyConfig := range r1.PagerdutyConfigs {
		if equal, reason := pagerDutyConfig.Equal(r2.PagerdutyConfigs[idx]); !equal {
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

var _ EqualityComparer[any] = (*RoutingTree)(nil)

// for our purposes we will only treat receivers and routes as opni config equality
func (r *RoutingTree) Equal(input any) (equal bool, reason string) {
	if _, ok := input.(*RoutingTree); !ok {
		return false, "input is not a routing tree"
	}
	other := input.(*RoutingTree)
	selfReceiverIndex := r.indexOpniReceivers()
	otherReceiverIndex := other.indexOpniReceivers()
	for id, r1 := range selfReceiverIndex {
		if r2, ok := otherReceiverIndex[id]; !ok {
			return false, fmt.Sprintf("configurations do not have matching receiver : %s", id)
		} else {
			if equal, reason := r1.Equal(r2); !equal {
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
