package types

type RepositoryType string

const (
	RepositoryTypeS3         RepositoryType = "s3"
	RepositoryTypeFileSystem RepositoryType = "fs"
)

type RepositoryRequest struct {
	Type     RepositoryType     `json:"type"`
	Settings RepositorySettings `json:"settings"`
}

type RepositorySettings struct {
	*S3Settings         `json:",inline,omitempty"`
	*FileSystemSettings `json:",inline,omitempty"`
}

type S3Settings struct {
	Bucket    string `json:"bucket"`
	Path      string `json:"base_path"`
	ReadOnly  bool   `json:"readonly"`
	ChunkSize string `json:"chunk_size,omitempty"`
}

type FileSystemSettings struct {
	Location string `json:"location"`
	Readonly bool   `json:"readonly"`
}

type SnapshotRequest struct {
	Indices            string `json:"indices,omitempty"`
	IgnoreUnavailable  bool   `json:"ignore_unavailable,omitempty"`
	IncludeGlobalState *bool  `json:"include_global_state,omitempty"`
	Partial            bool   `json:"partial,omitempty"`
}

type SnapshotResponse struct {
	UUID     string              `json:"uuid"`
	Indices  []string            `json:"indices"`
	State    SnapshotState       `json:"state"`
	Duration int                 `json:"duration_in_millis"`
	Failures []string            `json:"failures"`
	Shards   SnapshotShardStatus `json:"shards"`
}

type SnapshotState string

const (
	SnapshotStateInProgress SnapshotState = "IN_PROGRESS"
	SnapshotStateSuccess    SnapshotState = "SUCCESS"
	SnapshotStateFailed     SnapshotState = "FAILED"
	SnapshotStatePartial    SnapshotState = "PARTIAL"
)

type SnapshotShardStatus struct {
	Total      int `json:"total"`
	Failed     int `json:"failed"`
	Successful int `json:"successful"`
}

type SnapshotManagementRequest struct {
	Description    string           `json:"description,omitempty"`
	Enabled        *bool            `json:"enabled,omitempty"`
	SnapshotConfig SnapshotConfig   `json:"snapshot_config"`
	Creation       SnapshotCreation `json:"creation"`
	Deletion       SnapshotDeletion `json:"deletion,omitempty"`
	//TODO: Add notification options
}

type SnapshotConfig struct {
	SnapshotRequest    `json:",inline"`
	DateFormat         string            `json:"date_format,omitempty"`
	DateFormatTimezone string            `json:"date_format_timezone,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type SnapshotCreation struct {
	// Cron string
	Schedule  string `json:"schedule"`
	TimeLimit string `json:"time_limit,omitempty"`
}

type SnapshotDeletion struct {
	// Cron string
	Schedule  string                  `json:"schedule,omitempty"`
	TimeLimit string                  `json:"time_limit,omitempty"`
	Condition SnapshotDeleteCondition `json:"condition,omitempty"`
}

type SnapshotDeleteCondition struct {
	MaxCount int    `json:"max_count,omitempty"`
	MaxAge   string `json:"max_age,omitempty"`
	MinCount int    `json:"min_count,omitempty"`
}

type SnapshotManagementResponse struct {
	Version     int                       `json:"_version"`
	SeqNo       int                       `json:"_seq_no"`
	PrimaryTerm int                       `json:"_primary_term"`
	Policy      SnapshotManagementRequest `json:"sm_policy"`
}
