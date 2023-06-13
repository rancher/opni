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
)

var (
	LogEmbedding = LogEmbeddingSpec{
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
	ModelGroupID string `json:"model_group_id"`
	Status       int    `json:"status"`
}

type ModelSearchQuery struct {
	Query ModelQuery `json:"query"`
}

type ModelQuery struct {
	Match ModelQueryMatch `json:"match,omitempty"`
}

type ModelQueryMatch struct {
	Name string `json:"name,omitempty"`
}
type ModelGroupSearchResp struct {
	ModelGroupHits ModelGroupHits `json:"hits"`
}

type ModelGroupHits struct {
	Hits []ModelGroup `json:"hits"`
}

type ModelGroup struct {
	ID     string      `json:"_id"`
	Source ModelSource `json:"_source"`
}

type ModelSource struct {
	ModelID string `json:"model_id,omitempty"`
}

type GroupBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AccessMode  string `json:"access_mode"`
}

type ModelSpec struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Format       string `json:"model_format"`
	ModelGroupID string `json:"model_group_id"`
}

type ModelResp struct {
	TaskId string `json:"task_id"`
	Status string `json:"status"`
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
