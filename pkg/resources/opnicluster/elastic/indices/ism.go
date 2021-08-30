package indices

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	. "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"
)

type ISMApi struct {
	*elasticsearch.Client
	Policy *ISMPolicySpec
}

func (c *ISMApi) generatePath() strings.Builder {
	var path strings.Builder
	path.Grow(1 + len("_opendistro") + 1 + len("_ism") + 1 + len("policies") + 1 + len(c.Policy.PolicyId))
	path.WriteString("/")
	path.WriteString("_opendistro")
	path.WriteString("/")
	path.WriteString("_ism")
	path.WriteString("/")
	path.WriteString("policies")
	path.WriteString("/")
	path.WriteString(c.Policy.PolicyId)
	return path
}

func (c *ISMApi) GetISM(ctx context.Context) (*esapi.Response, error) {
	method := "GET"
	path := c.generatePath()

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

	return &esapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *ISMApi) CreateISM(ctx context.Context) (*esapi.Response, error) {
	var (
		method string
		body   io.Reader
	)
	method = "PUT"
	path := c.generatePath()

	marshalled, err := json.MarshalIndent(map[string]interface{}{
		"policy": c.Policy,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	body = bytes.NewReader(marshalled)

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

	return &esapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}

func (c *ISMApi) UpdateISM(ctx context.Context, seqNo int, primaryTerm int) (*esapi.Response, error) {
	var (
		method string
		body   io.Reader
	)
	method = "PUT"
	marshalled, err := json.Marshal(map[string]interface{}{
		"policy": c.Policy,
	})
	if err != nil {
		return nil, err
	}
	body = bytes.NewReader(marshalled)
	path := c.generatePath()

	req, err := http.NewRequest(method, path.String(), body)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}

	req.Header.Add(headerContentType, jsonContentHeader)

	query := req.URL.Query()
	query.Set("if_seq_no", fmt.Sprint(seqNo))
	query.Set("if_primary_term", fmt.Sprint(primaryTerm))
	req.URL.RawQuery = query.Encode()

	res, err := c.Perform(req)
	if err != nil {
		return nil, err
	}

	return &esapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil
}
