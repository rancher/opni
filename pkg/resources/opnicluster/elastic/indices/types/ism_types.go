package types

import "encoding/json"

type ISMPolicySpec struct {
	PolicyID          string            `json:"policy_id,omitempty"`
	Description       string            `json:"description"`
	ISMTemplate       *ISMTemplateSpec  `json:"ism_template,omitempty"`
	ErrorNotification *NotificationSpec `json:"error_notification"`
	DefaultState      string            `json:"default_state"`
	States            []StateSpec       `json:"states"`
}

type ISMTemplateSpec struct {
	IndexPatterns []string `json:"index_patterns"`
	Priority      int      `json:"priority,omitempty"`
}

type NotificationSpec struct {
	Destination     DestinationSpec `json:"destination"`
	MessageTemplate map[string]string
}

type DestinationSpec struct {
	DestinationType `json:",inline"`
}

type DestinationType struct {
	Chime         *ChimeSpec         `json:"chime,omitempty"`
	Slack         *SlackSpec         `json:"slack,omitempty"`
	CustomWebhook *CustomWebhookSpec `json:"custom_webhook,omitempty"`
}

type ChimeSpec struct {
	URL string `json:"url"`
}

type SlackSpec struct {
	URL string `json:"url"`
}
type CustomWebhookSpec struct {
	URL string `json:"url"`
}

type StateSpec struct {
	Name        string           `json:"name"`
	Actions     []ActionSpec     `json:"actions,omitempty"`
	Transitions []TransitionSpec `json:"transitions,omitempty"`
}

type ActionSpec struct {
	Timeout          string     `json:"timeout,omitempty"`
	Retry            *RetrySpec `json:"retry,omitempty"`
	*ActionOperation `json:",inline,omitempty"`
}

type RetrySpec struct {
	Count   int    `json:"count"`
	Backoff string `json:"backoff,omitempty"`
	Delay   string `json:"delay,omitempty"`
}

type ActionOperation struct {
	ForceMerge    *ForceMergeOperation    `json:"force_merge,omitempty"`
	ReadOnly      *ReadOnlyOperation      `json:"read_only,omitempty"`
	ReadWrite     *ReadWriteOperation     `json:"read_write,omitempty"`
	ReplicaCount  *ReplicaCountOperation  `json:"replica_count,omitempty"`
	Close         *CloseOperation         `json:"close,omitempty"`
	Open          *OpenOperation          `json:"open,omitempty"`
	Delete        *DeleteOperation        `json:"delete,omitempty"`
	Rollover      *RolloverOperation      `json:"rollover,omitempty"`
	Notification  *NotificationSpec       `json:"notification,omitempty"`
	Snapshot      *SnapshotOperation      `json:"snapshot,omitempty"`
	IndexPriority *IndexPriorityOperation `json:"index_priority,omitempty"`
	Allocation    *AllocationOperation    `json:"allocation,omitempty"`
}

type ForceMergeOperation struct {
	MaxNumSegments int `json:"max_num_segments"`
}

type ReadOnlyOperation struct {
}

type ReadWriteOperation struct {
}

type ReplicaCountOperation struct {
	NumberOfReplicas int `json:"number_of_replicas"`
}

type CloseOperation struct {
}

type OpenOperation struct {
}

type DeleteOperation struct {
}

type RolloverOperation struct {
	MinSize     string `json:"min_size,omitempty"`
	MinDocCount int    `json:"min_doc_count,omitempty"`
	MinIndexAge string `json:"min_index_age,omitempty"`
}

type SnapshotOperation struct {
	Repository   string `json:"repository"`
	SnapshotName string `json:"snapshot"`
}

type IndexPriorityOperation struct {
	Priority int `json:"priority"`
}

type AllocationOperation struct {
	Require map[string]string `json:"require,omitempty"`
	Include map[string]string `json:"include,omitempty"`
	Exclude map[string]string `json:"exclude,omitempty"`
	WaitFor string            `json:"wait_for,omitempty"`
}

type TransitionSpec struct {
	StateName  string         `json:"state_name"`
	Conditions *ConditionSpec `json:"conditions,omitempty"`
}

type ConditionSpec struct {
	MinIndexAge string    `json:"min_index_age,omitempty"`
	MinDocCount int       `json:"min_doc_count,omitempty"`
	MinSize     string    `json:"min_size,omitempty"`
	Cron        *CronSpec `json:"cron,omitempty"`
}

type CronSpec struct {
	Cron CronObject `json:"cron"`
}

type CronObject struct {
	Expression string `json:"expression"`
	Timezone   string `json:"timezone"`
}

type ISMGetResponse struct {
	ID          string        `json:"_id"`
	Version     int           `json:"_version"`
	SeqNo       int           `json:"_seq_no"`
	PrimaryTerm int           `json:"_primary_term"`
	Policy      ISMPolicySpec `json:"policy,omitempty"`
}

func (p ISMPolicySpec) MarshalJSON() ([]byte, error) {
	type policy ISMPolicySpec
	tmp := policy(p)
	tmp.PolicyID = ""
	return json.Marshal(tmp)
}
