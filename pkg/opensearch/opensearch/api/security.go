package api

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
)

type SecurityAPI struct {
	*opensearchtransport.Client
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

func (c *SecurityAPI) GetRole(ctx context.Context, name string) (*Response, error) {
	path := generateRolesPath(name)

	req, err := http.NewRequest(http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	return (*Response)(res), err
}

func (c *SecurityAPI) CreateRole(ctx context.Context, name string, body io.Reader) (*Response, error) {
	path := generateRolesPath(name)

	req, err := http.NewRequest(http.MethodPut, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add(headerContentType, jsonContentHeader)

	res, err := c.Perform(req)
	return (*Response)(res), err
}

func (c *SecurityAPI) DeleteRole(ctx context.Context, name string) (*Response, error) {
	path := generateRolesPath(name)

	req, err := http.NewRequest(http.MethodDelete, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	return (*Response)(res), err
}

func (c *SecurityAPI) GetUser(ctx context.Context, name string) (*Response, error) {
	path := generateUserPath(name)

	req, err := http.NewRequest(http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	return (*Response)(res), err
}

func (c *SecurityAPI) CreateUser(ctx context.Context, name string, body io.Reader) (*Response, error) {
	path := generateUserPath(name)

	req, err := http.NewRequest(http.MethodPut, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add(headerContentType, jsonContentHeader)

	res, err := c.Perform(req)
	return (*Response)(res), err
}

func (c *SecurityAPI) DeleteUser(ctx context.Context, name string) (*Response, error) {
	path := generateUserPath(name)

	req, err := http.NewRequest(http.MethodDelete, path.String(), nil)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	res, err := c.Perform(req)
	return (*Response)(res), err
}

func (c *SecurityAPI) GetRolesMapping(ctx context.Context, name string) (*Response, error) {
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
	return (*Response)(res), err
}

func (c *SecurityAPI) CreateRolesMapping(ctx context.Context, name string, body io.Reader) (*Response, error) {
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
	return (*Response)(res), err
}

func (c *SecurityAPI) DeleteRolesMapping(ctx context.Context, name string) (*Response, error) {
	method := "DELETE"
	path := generateRolesMappingPath(name)

	req, err := http.NewRequest(method, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	req.Header.Add(headerContentType, jsonContentHeader)

	res, err := c.Perform(req)
	return (*Response)(res), err
}
