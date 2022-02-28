package types

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
}

type TemplateMappingsSpec struct {
	Properties map[string]PropertySettings `json:"properties,omitempty"`
}

type PropertySettings struct {
	Type    string `json:"type,omitempty"`
	Format  string `json:"format,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}
