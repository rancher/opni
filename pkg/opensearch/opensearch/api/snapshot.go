package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type SnapshotAPI struct {
	*opensearchtransport.Client
}

func generateRepositoryPath(name string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_snapshot") + 1 + len(name))
	path.WriteString("/")
	path.WriteString("_snapshot")
	path.WriteString("/")
	path.WriteString(name)
	return path
}

func (a *SnapshotAPI) GetRepository(ctx context.Context, name string) (*Response, error) {
	method := http.MethodGet
	path := generateRepositoryPath(name)

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

func (a *SnapshotAPI) PutRepository(ctx context.Context, name string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := generateRepositoryPath(name)

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

func (a *SnapshotAPI) DeleteRepository(ctx context.Context, name string) (*Response, error) {
	method := http.MethodDelete
	path := generateRepositoryPath(name)

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
