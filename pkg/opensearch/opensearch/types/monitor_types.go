package types

import "encoding/json"

type Schedule struct {
	Period struct {
		Unit     string `json:"unit"`
		Interval int    `json:"interval"`
	} `json:"period"`
}

type Range struct {
	GTE    string `json:"gte"`
	LTE    string `json:"lte"`
	Format string `json:"format"`
}

type Query struct {
	Size         int             `json:"size"`
	Aggregations json.RawMessage `json:"aggregations"`
	Query        json.RawMessage `json:"query"`
}

type SearchInput struct {
	Search struct {
		Indices []string `json:"indices"`
		Query   Query    `json:"query"`
	} `json:"search"`
}

type MessageTemplate struct {
	Source string `json:"source"`
	Lang   string `json:"lang"`
}

type SubjectTemplate struct {
	Source string `json:"source"`
	Lang   string `json:"lang"`
}

type Action struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	DestinationID   string          `json:"destination_id"`
	MessageTemplate MessageTemplate `json:"message_template"`
	ThrottleEnabled bool            `json:"throttle_enabled"`
	SubjectTemplate SubjectTemplate `json:"subject_template"`
}

type Trigger struct {
	QueryLevelTrigger struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Severity  string    `json:"severity"`
		Condition Condition `json:"condition"`
		Actions   []Action  `json:"actions"`
	} `json:"query_level_trigger"`
}

type UIMetadata struct {
	Schedule struct {
		Timezone  interface{} `json:"timezone"`
		Frequency string      `json:"frequency"`
		Period    Schedule    `json:"period"`
		Daily     int         `json:"daily"`
		Weekly    struct {
			Tue  bool `json:"tue"`
			Wed  bool `json:"wed"`
			Thur bool `json:"thur"`
			Sat  bool `json:"sat"`
			Fri  bool `json:"fri"`
			Mon  bool `json:"mon"`
			Sun  bool `json:"sun"`
		} `json:"weekly"`
		Monthly struct {
			Type string `json:"type"`
			Day  int    `json:"day"`
		} `json:"monthly"`
		CronExpression string `json:"cronExpression"`
	} `json:"schedule"`
	MonitorType string `json:"monitor_type"`
	Search      struct {
		SearchType       string   `json:"searchType"`
		TimeField        string   `json:"timeField"`
		Aggregations     []string `json:"aggregations"`
		GroupBy          []string `json:"groupBy"`
		BucketValue      int      `json:"bucketValue"`
		BucketUnitOfTime string   `json:"bucketUnitOfTime"`
		Where            struct {
			FieldName       []string `json:"fieldName"`
			FieldRangeEnd   int      `json:"fieldRangeEnd"`
			FieldRangeStart int      `json:"fieldRangeStart"`
			FieldValue      string   `json:"fieldValue"`
			Operator        string   `json:"operator"`
		} `json:"where"`
	} `json:"search"`
}

type Condition struct {
	Script struct {
		Source string `json:"source"`
		Lang   string `json:"lang"`
	} `json:"script"`
}

type Monitor struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	MonitorType string        `json:"monitor_type"`
	Enabled     bool          `json:"enabled"`
	Schedule    Schedule      `json:"schedule"`
	Inputs      []SearchInput `json:"inputs"`
	Triggers    []Trigger     `json:"triggers"`
	// We omit UI Metadata here
}

type MonitorSpec struct {
	Id          string  `json:"_id"`
	Version     int     `json:"_version"`
	SeqNo       int     `json:"_seq_no"`
	PrimaryTerm int     `json:"_primary_term"`
	Monitor     Monitor `json:"monitor"`
}

type SlackConfig struct {
	URL string `json:"url"`
}

type WebhookConfig struct {
	URL          string            `json:"url"`
	HeaderParams map[string]string `json:"header_params"`
	Method       string            `json:"method"`
}

type Channel struct {
	ConfigID          string `json:"config_id"`
	LastUpdatedTimeMS int64  `json:"last_updated_time_ms"`
	CreatedTimeMS     int64  `json:"created_time_ms"`
	Config            struct {
		Name           string        `json:"name"`
		Description    string        `json:"description"`
		ConfigType     string        `json:"config_type"`
		IsEnabled      bool          `json:"is_enabled"`
		SlackDetails   SlackConfig   `json:"slack"`
		WebhookDetails WebhookConfig `json:"webhook"`
	} `json:"config"`
}

type ListChannelResponse struct {
	StartIndex       int       `json:"start_index"`
	TotalHits        int       `json:"total_hits"`
	TotalHitRelation string    `json:"total_hit_relation"`
	ChannelList      []Channel `json:"config_list"`
}

type AcknowledgeAlertRequest struct {
	Alerts []string `json:"alerts"`
}

type User struct {
	Name                 string   `json:"name"`
	BackendRoles         []string `json:"backend_roles"`
	Roles                []string `json:"roles"`
	CustomAttributeNames []string `json:"custom_attribute_names"`
	UserRequestedTenant  string   `json:"user_requested_tenant"`
}

type AlertHistory struct {
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

type ActionExecutionResult struct {
	ActionID          string `json:"action_id"`
	LastExecutionTime int64  `json:"last_execution_time"`
	ThrottledCount    int    `json:"throttled_count"`
}

type Alert struct {
	ID                    string                  `json:"id"`
	Version               int                     `json:"version"`
	MonitorID             string                  `json:"monitor_id"`
	SchemaVersion         int                     `json:"schema_version"`
	MonitorVersion        int                     `json:"monitor_version"`
	MonitorName           string                  `json:"monitor_name"`
	MonitorUser           User                    `json:"monitor_user"`
	TriggerID             string                  `json:"trigger_id"`
	TriggerName           string                  `json:"trigger_name"`
	State                 string                  `json:"state"`
	ErrorMessage          string                  `json:"error_message"`
	AlertHistory          []AlertHistory          `json:"alert_history"`
	Severity              string                  `json:"severity"`
	ActionExecutionResult []ActionExecutionResult `json:"action_execution_results"`
	StartTime             int64                   `json:"start_time"`
	LastNotificationTime  int64                   `json:"last_notification_time"`
	EndTime               *int64                  `json:"end_time"`
	AcknowledgedTime      *int64                  `json:"acknowledged_time"`
}

type ListAlertResponse struct {
	Alerts      []Alert `json:"alerts"`
	TotalAlerts int     `json:"totalAlerts"`
}
