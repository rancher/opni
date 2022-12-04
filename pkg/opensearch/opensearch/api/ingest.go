package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type IngestAPI struct {
	*opensearchtransport.Client
}

func generateIngestPipelinePath(pipelineID string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_ingest") + 1 + len("pipeline") + 1 + len(pipelineID))
	path.WriteString("/")
	path.WriteString("_ingest")
	path.WriteString("/")
	path.WriteString("pipeline")
	if pipelineID != "" {
		path.WriteString("/")
		path.WriteString(pipelineID)
	}
	return path
}

func (a *IngestAPI) GetIngestPipeline(ctx context.Context, pipelineID string) (*Response, error) {
	method := http.MethodGet
	path := generateIngestPipelinePath(pipelineID)

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

func (a *IngestAPI) PutIngestTemplate(ctx context.Context, pipelineID string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := generateIngestPipelinePath(pipelineID)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}

	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *IngestAPI) DeleteIngestPipeline(ctx context.Context, pipelineID string) (*Response, error) {
	method := http.MethodDelete
	path := generateIngestPipelinePath(pipelineID)

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
