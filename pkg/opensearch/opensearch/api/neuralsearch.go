package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
)

const (
	modelFrameworkBase  = "/_plugins/_ml/"
	clusterSettingsPath = "_cluster/settings"
)

type NeuralSearchAPI struct {
	*opensearchtransport.Client
}

func generateGroupSearchPath() strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("model_groups") + 1 + len("_search"))
	path.WriteString(modelFrameworkBase)
	path.WriteString("model_groups")
	path.WriteString("/")
	path.WriteString("_search")
	return path
}

func generateRegisterGroupPath() strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("model_groups") + 1 + len("_register"))
	path.WriteString(modelFrameworkBase)
	path.WriteString("model_groups")
	path.WriteString("/")
	path.WriteString("_register")
	return path
}

func generateModelSearchPath() strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("models") + 1 + len("_search"))
	path.WriteString(modelFrameworkBase)
	path.WriteString("models")
	path.WriteString("/")
	path.WriteString("_search")
	return path
}

func generateModelRegisterPath() strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("models") + 1 + len("_register"))
	path.WriteString(modelFrameworkBase)
	path.WriteString("models")
	path.WriteString("/")
	path.WriteString("_register")
	return path
}

func generateGetSearchLogsPath(index string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(index) + 1 + len("_search"))
	path.WriteString("/")
	path.WriteString(index)
	path.WriteString("/")
	path.WriteString("_search")
	return path
}

func generateModelDeployPath(modelID string) strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("models") + 1 + len(modelID) + 1 + len("_deploy"))
	path.WriteString(modelFrameworkBase)
	path.WriteString("models")
	path.WriteString("/")
	path.WriteString(modelID)
	path.WriteString("/")
	path.WriteString("_deploy")
	return path
}

func generateModelUnDeployPath(modelID string) strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("models") + 1 + len(modelID) + 1 + len("_undeploy"))
	path.WriteString(modelFrameworkBase)
	path.WriteString("models")
	path.WriteString("/")
	path.WriteString(modelID)
	path.WriteString("/")
	path.WriteString("_undeploy")
	return path
}

func generateModelDeletePath(modelID string) strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("models") + 1 + len(modelID))
	path.WriteString(modelFrameworkBase)
	path.WriteString("models")
	path.WriteString("/")
	path.WriteString(modelID)
	return path
}

func generateModelTaskStatusPath(taskID string) strings.Builder {
	var path strings.Builder
	path.Grow(len(modelFrameworkBase) + len("tasks") + 1 + len(taskID))
	path.WriteString(modelFrameworkBase)
	path.WriteString("tasks")
	path.WriteString("/")
	path.WriteString(taskID)
	return path
}

func generateGroupSearchBody() io.Reader {
	return opensearchutil.NewJSONReader(opensearchtypes.ModelGroupSearchBody)
}

func generateGroupRegisterBody() io.Reader {
	return opensearchutil.NewJSONReader(opensearchtypes.ModelGroupRegisterBody)
}

func generateModelSearchBody() io.Reader {
	return opensearchutil.NewJSONReader(opensearchtypes.ModelSearchBody)
}

func generateModelRegisterBody(groupID string, customUrl string) io.Reader {
	modelBody := &opensearchtypes.ModelSpec{
		Name:         opensearchtypes.ModelName,
		Version:      opensearchtypes.ModelVersion,
		Format:       opensearchtypes.ModelFormat,
		ModelGroupID: groupID,
	}

	if customUrl != "" {
		modelBody.Url = customUrl
	}

	return opensearchutil.NewJSONReader(modelBody)
}

func generateGetSearchLogsBody(query string, modelID string, results int) io.Reader {
	queryBody := &opensearchtypes.LogSearchQuery{
		Size: results,
		Query: opensearchtypes.NeuralSearch{
			Neural: opensearchtypes.Neural{
				LogEmbedding: opensearchtypes.LogEmbeddingQuery{
					QueryText: query,
					ModelID:   modelID,
					K:         results,
				},
			},
		},
		Source: []string{"log"},
	}

	return opensearchutil.NewJSONReader(queryBody)
}

func generateEnableModelAccessControlBody() io.Reader {
	return opensearchutil.NewJSONReader(opensearchtypes.EnableMlAccessControl)
}

func (a *NeuralSearchAPI) PostEnableModelAccessControl(ctx context.Context) (*Response, error) {
	method := http.MethodPost
	path := clusterSettingsPath
	body := generateEnableModelAccessControlBody()

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) PostSearchExistingModelGroup(ctx context.Context) (*Response, error) {
	method := http.MethodPost
	path := generateGroupSearchPath()
	body := generateGroupSearchBody()

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) PostRegisterModelGroup(ctx context.Context) (*Response, error) {
	method := http.MethodPost
	path := generateRegisterGroupPath()
	body := generateGroupRegisterBody()

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) PostSearchExistingModel(ctx context.Context) (*Response, error) {
	method := http.MethodPost
	path := generateModelSearchPath()
	body := generateModelSearchBody()

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) PostRegisterModel(ctx context.Context, groupID string, customUrl string) (*Response, error) {
	method := http.MethodPost
	path := generateModelRegisterPath()
	body := generateModelRegisterBody(groupID, customUrl)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) GetModelTaskStatus(ctx context.Context, taskID string) (*Response, error) {
	method := http.MethodGet
	path := generateModelTaskStatusPath(taskID)

	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) PostDeployModel(ctx context.Context, modelID string) (*Response, error) {
	method := http.MethodPost
	path := generateModelDeployPath(modelID)
	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) GetSearchLogs(ctx context.Context, index string, query string, modelID string, results int) (*Response, error) {
	method := http.MethodGet
	path := generateGetSearchLogsPath(index)
	body := generateGetSearchLogsBody(query, modelID, results)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) MaybeCreateModelGroup(ctx context.Context) (string, error) {
	resp, err := a.PostSearchExistingModelGroup(ctx)
	if err != nil {
		return "", err
	}
	groupSearchResp := opensearchtypes.ModelGroupSearchResp{}
	err = json.NewDecoder(resp.Body).Decode(&groupSearchResp)
	if err != nil {
		return "", err
	}

	groupExists := len(groupSearchResp.ModelGroupHits.Hits) > 0
	if groupExists {
		return groupSearchResp.ModelGroupHits.Hits[0].ID, nil
	}

	resp, err = a.PostRegisterModelGroup(ctx)
	if err != nil {
		return "", err
	}
	registerModelResp := types.ModelGroupRegisterResp{}
	err = json.NewDecoder(resp.Body).Decode(&registerModelResp)
	if err != nil {
		return "", err
	}

	if registerModelResp.ModelGroupID == "" {
		return "", fmt.Errorf("group id is nil: group register resp %v", resp.String())
	}
	return registerModelResp.ModelGroupID, nil
}

func (a *NeuralSearchAPI) MaybeCreateRegisteredModel(ctx context.Context, groupID string, customUrl string) (string, error) {
	resp, err := a.PostSearchExistingModel(ctx)
	if err != nil {
		return "", err
	}

	modelSearchResp := types.ModelGroupSearchResp{}
	err = json.NewDecoder(resp.Body).Decode(&modelSearchResp)
	if err != nil {
		return "", err
	}
	modelUploaded := len(modelSearchResp.ModelGroupHits.Hits) > 0
	if modelUploaded {
		return modelSearchResp.ModelGroupHits.Hits[0].Source.ModelID, nil
	}
	resp, err = a.PostRegisterModel(ctx, groupID, customUrl)
	if err != nil {
		return "", err
	}
	uploadRes := types.ModelResp{}
	err = json.NewDecoder(resp.Body).Decode(&uploadRes)
	if err != nil {
		return "", err
	}
	uploadTaskID := uploadRes.TaskId

	resp, err = a.GetModelTaskStatus(ctx, uploadTaskID)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return "", fmt.Errorf("failed to register model: %s", resp.String())
	}
	modelStatusResp := types.ModelTaskStatus{}
	err = json.NewDecoder(resp.Body).Decode(&modelStatusResp)
	if err != nil {
		return "", err
	}
	modelID := modelStatusResp.ModelID
	if modelStatusResp.State != types.ModelTaskStatusCompleted {
		return "", fmt.Errorf("model not uploaded to opensearch: %s", resp.String())
	}
	return modelID, nil
}

func (a *NeuralSearchAPI) DeployNeuralSearchModel(ctx context.Context, modelID string) error {
	resp, err := a.PostDeployModel(ctx, modelID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to load model into memory: %s", resp.String())
	}

	loadRes := types.ModelResp{}
	err = json.NewDecoder(resp.Body).Decode(&loadRes)
	if err != nil {
		return err
	}
	loadTaskID := loadRes.TaskId

	statusRes, err := a.GetModelTaskStatus(ctx, loadTaskID)
	if err != nil {
		return err
	}
	modelStatusResp := types.ModelTaskStatus{}
	err = json.NewDecoder(statusRes.Body).Decode(&modelStatusResp)
	if err != nil {
		return err
	}

	if modelStatusResp.State == types.ModelTaskStatusFailed {
		return fmt.Errorf("model %s failed to deploy: %s", modelID, statusRes.String())
	}
	return nil
}

func (a *NeuralSearchAPI) PostUnDeployModel(ctx context.Context, modelID string) (*Response, error) {
	method := http.MethodPost
	path := generateModelUnDeployPath(modelID)
	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *NeuralSearchAPI) PostDeleteModel(ctx context.Context, modelID string) (*Response, error) {
	method := http.MethodDelete
	path := generateModelDeletePath(modelID)
	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}
