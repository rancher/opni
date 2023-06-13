package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type IndicesAPI struct {
	*opensearchtransport.Client
}

func generateCatIndicesPath(indices []string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_cat") + 1 + len("indices") + 1 + len(strings.Join(indices, ",")))
	path.WriteString("/")
	path.WriteString("_cat")
	path.WriteString("/")
	path.WriteString("indices")
	if len(indices) > 0 {
		path.WriteString("/")
		path.WriteString(strings.Join(indices, ","))
	}
	return path
}

func generateGetIndexTemplatesPath(names []string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_index_template") + 1 + len(strings.Join(names, ",")))
	path.WriteString("/")
	path.WriteString("_index_template")
	if len(names) > 0 {
		path.WriteString("/")
		path.WriteString(strings.Join(names, ","))
	}
	return path
}

func generateIndicesPath(names []string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(strings.Join(names, ",")))
	path.WriteString("/")
	path.WriteString(strings.Join(names, ","))
	return path
}

func generateDocumentPath(index, documentID string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(index) + 1 + len("_doc") + 1 + len(documentID))
	path.WriteString("/")
	path.WriteString(index)
	path.WriteString("/_doc")
	path.WriteString("/")
	path.WriteString(documentID)
	return path
}

func generateDocumentUpdatePath(index, documentID string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(index) + 1 + len("_update") + 1 + len(documentID))
	path.WriteString("/")
	path.WriteString(index)
	path.WriteString("/")
	path.WriteString("_update")
	path.WriteString("/")
	path.WriteString(documentID)
	return path
}

func generateIndicesSettingsPath(indices []string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(strings.Join(indices, ",")) + 1 + len("_settings"))
	if len(indices) > 0 {
		path.WriteString("/")
		path.WriteString(strings.Join(indices, ","))
	}
	path.WriteString("/")
	path.WriteString("_settings")
	return path
}

func generateIndicesMappingsPath(indices []string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(strings.Join(indices, ",")) + 1 + len("_mappings"))
	if len(indices) > 0 {
		path.WriteString("/")
		path.WriteString(strings.Join(indices, ","))
	}
	path.WriteString("/")
	path.WriteString("_mappings")
	return path
}

func generateDeleteByQueryPath(indices []string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len(strings.Join(indices, ",")) + 1 + len("_delete_by_query"))
	path.WriteString("/")
	path.WriteString(strings.Join(indices, ","))
	path.WriteString("/")
	path.WriteString("_delete_by_query")
	return path
}

func (a *IndicesAPI) CatIndices(ctx context.Context, indices []string) (*Response, error) {
	method := http.MethodGet
	path := generateCatIndicesPath(indices)

	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("format", "json")
	req.URL.RawQuery = q.Encode()

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *IndicesAPI) GetIndexTemplates(ctx context.Context, names []string) (*Response, error) {
	method := http.MethodGet
	path := generateGetIndexTemplatesPath(names)

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

func (a *IndicesAPI) PutIndexTemplate(ctx context.Context, name string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := generateGetIndexTemplatesPath([]string{name})

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

func (a *IndicesAPI) DeleteIndexTemplate(ctx context.Context, name string) (*Response, error) {
	method := http.MethodDelete
	path := generateGetIndexTemplatesPath([]string{name})

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

func (a *IndicesAPI) CreateIndex(ctx context.Context, name string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := "/" + name

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

func (a *IndicesAPI) DeleteIndices(ctx context.Context, names []string) (*Response, error) {
	method := http.MethodDelete
	path := generateIndicesPath(names)

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

func (a *IndicesAPI) DeleteByID(ctx context.Context, index, id string) (*Response, error) {
	method := http.MethodDelete
	path := generateDocumentPath(index, id)

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

func (a *IndicesAPI) AsyncDeleteByQuery(ctx context.Context, indices []string, body io.Reader) (*Response, error) {
	method := http.MethodPost
	path := generateDeleteByQueryPath(indices)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("wait_for_completion", "false")
	q.Set("refresh", "true")
	q.Set("search_type", "dfs_query_then_fetch")
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

func (a *IndicesAPI) UpdateIndicesSettings(ctx context.Context, indices []string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := generateIndicesSettingsPath(indices)

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

func (a *IndicesAPI) UpdateIndicesMappings(ctx context.Context, indices []string, body io.Reader) (*Response, error) {
	method := http.MethodPut
	path := generateIndicesMappingsPath(indices)

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

func (a *IndicesAPI) UpdateAlias(ctx context.Context, body io.Reader) (*Response, error) {
	method := http.MethodPost
	path := "/_aliases"

	req, err := http.NewRequest(method, path, body)
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

func (a *IndicesAPI) SynchronousReindex(ctx context.Context, body io.Reader) (*Response, error) {
	method := http.MethodPost
	path := "/_reindex"

	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("wait_for_completion", "true")
	req.URL.RawQuery = q.Encode()
	if ctx != nil {
		req = req.WithContext(ctx)
	}

	req.Header.Add(headerContentType, jsonContentHeader)
	res, err := a.Perform(req)
	return (*Response)(res), err
}

func (a *IndicesAPI) GetDocument(ctx context.Context, index, documentID string) (*Response, error) {
	method := http.MethodGet
	path := generateDocumentPath(index, documentID)

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

func (a *IndicesAPI) UpdateDocument(ctx context.Context, index, documentID string, body io.Reader) (*Response, error) {
	method := http.MethodPost
	path := generateDocumentUpdatePath(index, documentID)

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

func (a *IndicesAPI) AddDocument(ctx context.Context, index, documentID string, body io.Reader) (*Response, error) {
	method := http.MethodPost
	path := generateDocumentPath(index, documentID)

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
