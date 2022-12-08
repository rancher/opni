package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type ClusterAPI struct {
	*opensearchtransport.Client
}

func generateClusterHealthPath() strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_cluster") + 1 + len("health"))
	path.WriteString("/")
	path.WriteString("_cluster")
	path.WriteString("/")
	path.WriteString("health")
	return path
}

func (a *ClusterAPI) GetClusterHealth(ctx context.Context) (*Response, error) {
	method := http.MethodGet
	path := generateClusterHealthPath()

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
