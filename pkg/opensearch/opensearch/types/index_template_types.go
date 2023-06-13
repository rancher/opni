package types

import (
	"encoding/json"
	"strconv"

	"github.com/tidwall/gjson"
)

type IndexTemplateSpec struct {
	TemplateName  string       `json:"-"`
	IndexPatterns []string     `json:"index_patterns,omitempty"`
	Template      TemplateSpec `json:"template,omitempty"`
	Priority      int          `json:"priority,omitempty"`
}

type TemplateSpec struct {
	Settings TemplateSettingsSpec `json:"settings,omitempty"`
	Mappings TemplateMappingsSpec `json:"mappings,omitempty"`
}

type TemplateSettingsSpec struct {
	NumberOfShards   int    `json:"number_of_shards,omitempty"`
	NumberOfReplicas int    `json:"number_of_replicas,omitempty"`
	ISMPolicyID      string `json:"opendistro.index_state_management.policy_id,omitempty"`
	RolloverAlias    string `json:"opendistro.index_state_management.rollover_alias,omitempty"`
	DefaultPipeline  string `json:"default_pipeline,omitempty"`
	KNN              bool   `json:"knn,omitempty"`
}

type TemplateMappingsSpec struct {
	DateDetection    *bool                            `json:"date_detection,omitempty"`
	DynamicTemplates []map[string]DynamicTemplateSpec `json:"dynamic_templates,omitempty"`
	Properties       map[string]PropertySettings      `json:"properties,omitempty"`
}

type PropertySettings struct {
	Type        string                      `json:"type,omitempty"`
	Format      string                      `json:"format,omitempty"`
	Enabled     *bool                       `json:"enabled,omitempty"`
	Fields      map[string]FieldsSpec       `json:"fields,omitempty"`
	IgnoreAbove int                         `json:"ignore_above,omitempty"`
	Properties  map[string]PropertySettings `json:"properties,omitempty"`
}

type DynamicTemplateSpec struct {
	MatchMappingType string           `json:"match_mapping_type,omitempty"`
	Match            string           `json:"match,omitempty"`
	UnMatch          string           `json:"unmatch,omitempty"`
	PathMatch        string           `json:"path_match,omitempty"`
	PathUnMatch      string           `json:"path_unmatch,omitempty"`
	Mapping          PropertySettings `json:"mapping,omitempty"`
}

type FieldsSpec struct {
	Type        string `json:"type,omitempty"`
	IgnoreAbove int    `json:"ignore_above,omitempty"`
}

type GetIndexTemplateObject struct {
	Name     string            `json:"name,omitempty"`
	Template IndexTemplateSpec `json:"index_template,omitempty"`
}

type GetIndexTemplateResponse struct {
	IndexTemplates []GetIndexTemplateObject `json:"index_templates,omitempty"`
}

func (t *TemplateSettingsSpec) UnmarshalJSON(data []byte) error {
	value := gjson.GetBytes(data, "index")
	var err error
	if value.Exists() {
		t.DefaultPipeline = value.Get("default_pipeline").String()
		t.RolloverAlias = value.Get("opendistro.index_state_management.rollover_alias").String()
		t.ISMPolicyID = value.Get("opendistro.index_state_management.policy_id").String()

		numReplicas := value.Get("number_of_replicas")
		if numReplicas.Exists() {
			t.NumberOfReplicas, err = strconv.Atoi(numReplicas.String())
			if err != nil {
				return err
			}
		}
		numShards := value.Get("number_of_shards")
		if numShards.Exists() {
			t.NumberOfShards, err = strconv.Atoi(value.Get("number_of_shards").String())
			if err != nil {
				return err
			}
		}
		return nil
	}

	type settings TemplateSettingsSpec
	tmp := settings{}
	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

	t.DefaultPipeline = tmp.DefaultPipeline
	t.RolloverAlias = tmp.RolloverAlias
	t.ISMPolicyID = tmp.ISMPolicyID
	t.NumberOfReplicas = tmp.NumberOfReplicas
	t.NumberOfShards = tmp.NumberOfShards
	return nil
}
