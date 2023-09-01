package types

const (
	ModelTaskStatusCreated   = "CREATED"
	ModelTaskStatusCompleted = "COMPLETED"
	ModelTaskStatusFailed    = "FAILED"
	ModelName                = "huggingface/sentence-transformers/all-distilroberta-v1"
	ModelVersion             = "1.0.1"
	ModelFormat              = "TORCH_SCRIPT"
	LogEmbeddingName         = "log_embedding"
	ModelGroupName           = "opni-neural-search-model-group"
	ModelGroupDesc           = "model group for neural search"
	ModelAccess              = "private"
	DefaultSearchResultSize  = 10
)

var (
	LogEmbeddingMappings = LogEmbeddingSpec{
		Type:      "knn_vector",
		Dimension: 768,
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
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	Format       string `json:"model_format,omitempty"`
	ModelGroupID string `json:"model_group_id,omitempty"`
	Url          string `json:"url,omitempty"`
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
