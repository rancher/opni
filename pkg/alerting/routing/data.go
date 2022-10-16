package routing

import (
	"fmt"
	cfg "github.com/prometheus/alertmanager/config"
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
// RoutingTree must always marshal to AlertManager's config, otherwise backend will reject it
type RoutingTree struct {
	Global       *GlobalConfig      `yaml:"global,omitempty" json:"global,omitempty"`
	Route        *cfg.Route         `yaml:"route,omitempty" json:"route,omitempty"`
	InhibitRules []*cfg.InhibitRule `yaml:"inhibit_rules,omitempty" json:"inhibit_rules,omitempty"`
	Receivers    []*Receiver        `yaml:"receivers,omitempty" json:"receivers,omitempty"`
	Templates    []string           `yaml:"templates" json:"templates"`
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
			return fmt.Errorf("(conditionId, endpointId) key already exists and should not")
		}
	}
	return nil
}

func (o *OpniInternalRouting) GetFromCondition(conditionId string) (map[string]*OpniRoutingMetadata, error) {
	if res, ok := o.Content[conditionId]; !ok {
		return nil, fmt.Errorf("conditionNotFound")
	} else {
		return res, nil
	}
}

func (o *OpniInternalRouting) Get(conditionId, endpointId string) (*OpniRoutingMetadata, error) {
	if condition, ok := o.Content[conditionId]; !ok {
		return nil, fmt.Errorf("conditionNotFound")
	} else {
		if metadata, ok := condition[endpointId]; !ok {
			return nil, fmt.Errorf("endpointNotFound")
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
		return fmt.Errorf("conditionNotFound")
	}
	if _, ok := o.Content[conditionId][notificationId]; !ok {
		return fmt.Errorf("endpointNotFound")
	}
	o.Content[conditionId][notificationId] = &OpniRoutingMetadata{
		EndpointType: metadata.EndpointType,
		Position:     metadata.Position,
	}
	return nil
}

func (o *OpniInternalRouting) RemoveEndpoint(conditionId string, endpointId string) error {
	if _, ok := o.Content[conditionId]; !ok {
		return fmt.Errorf("conditionNotFound")
	}
	if _, ok := o.Content[conditionId][endpointId]; !ok {
		return fmt.Errorf("endpointNotFound")
	}
	delete(o.Content[conditionId], endpointId)
	return nil
}

func (o *OpniInternalRouting) RemoveCondition(conditionId string) error {
	if _, ok := o.Content[conditionId]; !ok {
		return fmt.Errorf("conditionNotFound")
	} else {
		delete(o.Content, conditionId)
	}
	return nil
}
