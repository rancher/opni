package opensearch

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

type SecurityAPI struct {
	*opensearch.Client
}

func generateRolesPath(name string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_plugins") + 1 + len("_security") + 1 + len("api") + 1 + len("roles") + 1 + len(name))
	path.WriteString("/")
	path.WriteString("_plugins")
	path.WriteString("/")
	path.WriteString("_security")
	path.WriteString("/")
	path.WriteString("api")
	path.WriteString("/")
	path.WriteString("roles")
	path.WriteString("/")
	path.WriteString(name)
	return path
}

func generateUserPath(name string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_plugins") + 1 + len("_security") + 1 + len("api") + 1 + len("internalusers") + 1 + len(name))
	path.WriteString("/")
	path.WriteString("_plugins")
	path.WriteString("/")
	path.WriteString("_security")
	path.WriteString("/")
	path.WriteString("api")
	path.WriteString("/")
	path.WriteString("internalusers")
	path.WriteString("/")
	path.WriteString(name)
	return path
}

func generateRolesMappingPath(name string) strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_plugins") + 1 + len("_security") + 1 + len("api") + 1 + len("rolesmapping") + 1 + len(name))
	path.WriteString("/")
	path.WriteString("_plugins")
	path.WriteString("/")
	path.WriteString("_security")
	path.WriteString("/")
	path.WriteString("api")
	path.WriteString("/")
	path.WriteString("rolesmapping")
	path.WriteString("/")
	path.WriteString(name)
	return path
}

func (c *SecurityAPI) GetRole(ctx context.Context, name string) (*opensearchapi.Response, error) {
	method := "GET"
	path := generateRolesPath(name)

	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *SecurityAPI) CreateRole(ctx context.Context, name string, body io.Reader) (*opensearchapi.Response, error) {
	method := "PUT"
	path := generateRolesPath(name)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add(headerContentType, jsonContentHeader)

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *SecurityAPI) GetUser(ctx context.Context, name string) (*opensearchapi.Response, error) {
	method := "GET"
	path := generateUserPath(name)

	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *SecurityAPI) CreateUser(ctx context.Context, name string, body io.Reader) (*opensearchapi.Response, error) {
	method := "PUT"
	path := generateUserPath(name)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add(headerContentType, jsonContentHeader)

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *SecurityAPI) GetRolesMapping(ctx context.Context, name string) (*opensearchapi.Response, error) {
	method := "GET"
	path := generateRolesMappingPath(name)

	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *SecurityAPI) CreateRolesMapping(ctx context.Context, name string, body io.Reader) (*opensearchapi.Response, error) {
	method := "PUT"
	path := generateRolesMappingPath(name)

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add(headerContentType, jsonContentHeader)

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}
