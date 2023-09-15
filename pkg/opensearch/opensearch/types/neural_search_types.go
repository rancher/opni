package types

const (
	ModelTaskStatusCreated   = "CREATED"
	ModelTaskStatusCompleted = "COMPLETED"
	ModelTaskStatusFailed    = "FAILED"
	ModelGroupName           = "opni-neural-search-model-group"
	ModelGroupDesc           = "model group for neural search"
	ModelAccess              = "private"
	DefaultSearchResultSize  = 10
	// below parameters from the table in https://opensearch.org/docs/latest/ml-commons-plugin/pretrained-models/
	ModelName             = "huggingface/sentence-transformers/all-distilroberta-v1"
	ModelVersion          = "1.0.1"
	ModelFormat           = "TORCH_SCRIPT"
	ModelTypeBert         = "roberta"
	LogEmbeddingName      = "log_embedding"
	EmbeddingDimension    = 768
	FrameworkType         = "sentence_transformers"
	ModelContentHashValue = "92bc10216c720b57a6bab1d7ca2cc2e559156997212a7f0d8bb70f2edfedc78b"
)

var (
	LogEmbeddingMappings = LogEmbeddingSpec{
		Type:      "knn_vector",
		Dimension: EmbeddingDimension,
		Method: MethodSpec{
			Name:      "hnsw",
			SpaceType: "l2",
			Engine:    "nmslib",
			Parameters: SearchParamSpec{
				EfConstruction: 128,
				M:              24,
			},
		},
	}

	EnableMlAccessControl = MlSettings{
		Transient: TransientMlSettings{
			ModelAccessControlEnabled: true,
		},
	}

	EnableRegisterViaUrl = MlSettings{
		Transient: TransientMlSettings{
			RegisterViaUrlEnabled: true,
		},
	}

	ModelGroupSearchBody = ModelSearchQuery{
		Query: ModelQuery{
			Match: ModelQueryMatch{
				Name: ModelGroupName,
			},
		},
	}

	ModelGroupRegisterBody = GroupBody{
		Name:        ModelGroupName,
		Description: ModelGroupDesc,
		AccessMode:  ModelAccess,
	}

	ModelSearchBody = ModelSearchQuery{
		Query: ModelQuery{
			Match: ModelQueryMatch{
				Name: ModelName,
			},
		},
	}
)

type LogEmbeddingSpec struct {
	Type      string     `json:"type,omitempty"`
	Dimension int        `json:"dimension,omitempty"`
	Method    MethodSpec `json:"method,omitempty"`
}

type MlSettings struct {
	Transient TransientMlSettings `json:"transient,omitempty"`
}

type TransientMlSettings struct {
	ModelAccessControlEnabled bool `json:"plugins.ml_commons.model_access_control_enabled,omitempty"`
	RegisterViaUrlEnabled     bool `json:"plugins.ml_commons.allow_registering_model_via_url,omitempty"`
}

type MethodSpec struct {
	Name       string          `json:"name,omitempty"`
	SpaceType  string          `json:"space_type,omitempty"`
	Engine     string          `json:"engine,omitempty"`
	Parameters SearchParamSpec `json:"parameters,omitempty"`
}

type SearchParamSpec struct {
	EfConstruction int `json:"ef_construction,omitempty"`
	M              int `json:"m,omitempty"`
}

type ModelGroupRegisterResp struct {
	ModelGroupID string `json:"model_group_id,omitempty"`
	Status       int    `json:"status,omitempty"`
}

type ModelSearchQuery struct {
	Query ModelQuery `json:"query,omitempty"`
}

type ModelQuery struct {
	Match ModelQueryMatch `json:"match,omitempty"`
}

type ModelQueryMatch struct {
	Name string `json:"name,omitempty"`
}
type ModelGroupSearchResp struct {
	ModelGroupHits ModelGroupHits `json:"hits,omitempty"`
}

type ModelGroupHits struct {
	Hits []ModelGroup `json:"hits,omitempty"`
}

type ModelGroup struct {
	ID     string      `json:"_id,omitempty"`
	Source ModelSource `json:"_source,omitempty"`
}

type ModelSource struct {
	ModelID string `json:"model_id,omitempty"`
}

type GroupBody struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	AccessMode  string `json:"access_mode,omitempty"`
}

type ModelSpec struct {
	Name                  string      `json:"name,omitempty"`
	Version               string      `json:"version,omitempty"`
	Format                string      `json:"model_format,omitempty"`
	ModelGroupID          string      `json:"model_group_id,omitempty"`
	ModelContentHashValue string      `json:"model_content_hash_value,omitempty"`
	ModelConfig           ModelConfig `json:"model_config,omitempty"`
	CustomUrl             string      `json:"url,omitempty"`
}

type ModelConfig struct {
	ModelType     string `json:"model_type,omitempty"`
	Dimension     int    `json:"embedding_dimension,omitempty"`
	FrameworkType string `json:"framework_type,omitempty"`
}

type ModelResp struct {
	TaskId string `json:"task_id,omitempty"`
	Status string `json:"status,omitempty"`
}

type ModelTaskStatus struct {
	ModelID        string   `json:"model_id,omitempty"`
	TaskType       string   `json:"task_type,omitempty"`
	FuncName       string   `json:"function_name,omitempty"`
	State          string   `json:"state,omitempty"`
	WorkerNode     []string `json:"worker_node,omitempty"`
	CreateTime     int      `json:"create_time,omitempty"`
	LastUpdateTime int      `json:"last_update_time,omitempty"`
	IsAsync        bool     `json:"is_async,omitempty"`
}

type LogSearchQuery struct {
	Size   int          `json:"size,omitempty"`
	Query  NeuralSearch `json:"query,omitempty"`
	Source []string     `json:"_source,omitempty"`
}

type NeuralSearch struct {
	Neural Neural `json:"neural,omitempty"`
}

type Neural struct {
	LogEmbedding LogEmbeddingQuery `json:"log_embedding,omitempty"`
}

type LogEmbeddingQuery struct {
	QueryText string `json:"query_text,omitempty"`
	ModelID   string `json:"model_id,omitempty"`
	K         int    `json:"k,omitempty"`
}

type LogSearchResponse struct {
	LogHits LogHits `json:"hits,omitempty"`
}

type LogHits struct {
	Hits []LogResult `json:"hits,omitempty"`
}

type LogResult struct {
	Index  string    `json:"_index,omitempty"`
	ID     string    `json:"_id,omitempty"`
	Source LogSource `json:"_source,omitempty"`
}

type LogSource struct {
	Log string `json:"log,omitempty"`
}
