package api

import (
	"context"
	"io"
	"net/http"
	"strconv"
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

func generateSnapshotPath(name, repository string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_snapshot") + 1 + len(repository) + 1 + len(name))
	path.WriteString("/")
	path.WriteString("_snapshot")
	path.WriteString("/")
	path.WriteString(repository)
	path.WriteString("/")
	path.WriteString(name)
	return path
}

func generateSnapshotPolicyPath(name string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_plugins") + 1 + len("_sm") + 1 + len("policies") + 1 + len(name))
	path.WriteString("/")
	path.WriteString("_plugins")
	path.WriteString("/")
	path.WriteString("_sm")
	path.WriteString("/")
	path.WriteString("policies")
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

func (a *SnapshotAPI) CreateSnapshot(ctx context.Context, name, repository string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := generateSnapshotPath(name, repository)

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

func (a *SnapshotAPI) GetSnapshot(ctx context.Context, name, repository string) (*Response, error) {
	method := http.MethodGet
	path := generateSnapshotPath(name, repository)

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

func (a *SnapshotAPI) DeleteSnapshot(ctx context.Context, name, repository string) (*Response, error) {
	method := http.MethodDelete
	path := generateSnapshotPath(name, repository)

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

func (a *SnapshotAPI) CreateSnapshotPolicy(ctx context.Context, name string, body io.Reader) (*Response, error) {
	method := http.MethodPost
	path := generateSnapshotPolicyPath(name)

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

func (a *SnapshotAPI) UpdateSnapshotPolicy(
	ctx context.Context,
	name string,
	seqNo int,
	primaryTerm int,
	body io.Reader,
) (*Response, error) {
	method := http.MethodPut
	path := generateSnapshotPolicyPath(name)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("if_seq_no", strconv.Itoa(seqNo))
	q.Set("if_primary_term", strconv.Itoa(primaryTerm))
	req.URL.RawQuery = q.Encode()

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	if body != nil {
		req.Header.Add(headerContentType, jsonContentHeader)
	}
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *SnapshotAPI) GetSnapshotPolicy(ctx context.Context, name string) (*Response, error) {
	method := http.MethodGet
	path := generateSnapshotPolicyPath(name)

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

func (a *SnapshotAPI) DeleteSnapshotPolicy(ctx context.Context, name string) (*Response, error) {
	method := http.MethodDelete
	path := generateSnapshotPolicyPath(name)

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
