package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type TasksAPI struct {
	*opensearchtransport.Client
}

func generateTasksPath(id string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_tasks") + 1 + len(id))
	path.WriteString("/")
	path.WriteString("_tasks")
	path.WriteString("/")
	path.WriteString(id)
	return path
}

func (a *TasksAPI) GetTask(ctx context.Context, id string) (*Response, error) {
	method := http.MethodGet
	path := generateTasksPath(id)

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
