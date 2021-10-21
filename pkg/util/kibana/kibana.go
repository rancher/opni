package kibana

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/opensearch-project/opensearch-go/opensearchapi"
)

const (
	headerContentType        = "Content-Type"
	kibanaCrossHeaderType    = "kbn-xsrf"
	securityTenantHeaderType = "securitytenant"
)

type Client struct {
	*http.Client
	url *url.URL
}

type Config struct {
	Username  string
	Password  string
	URL       string
	Transport http.RoundTripper
}

func NewClient(cfg Config) (*Client, error) {
	newURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	newURL.User = url.UserPassword(cfg.Username, cfg.Password)

	var client *http.Client

	if cfg.Transport == nil {
		client = &http.Client{}
	} else {
		client = &http.Client{
			Transport: cfg.Transport,
		}
	}

	return &Client{
		Client: client,
		url:    newURL,
	}, nil
}

func (c *Client) generateObjectPath() string {
	var path strings.Builder
	path.Grow(len("api") + 1 + len("saved_objects") + 1 + len("_import"))
	path.WriteString("api")
	path.WriteString("/")
	path.WriteString("saved_objects")
	path.WriteString("/")
	path.WriteString("_import")

	return path.String()
}

func (c *Client) ImportObjects(ctx context.Context, objectData string, objectName string) (*opensearchapi.Response, error) {
	method := "POST"
	relPath := &url.URL{Path: c.generateObjectPath()}
	absURL := c.url.ResolveReference(relPath)

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", objectName)
	if err != nil {
		return nil, err
	}
	part.Write([]byte(objectData))
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, absURL.String(), body)
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	req.Header.Add(headerContentType, writer.FormDataContentType())
	req.Header.Add(kibanaCrossHeaderType, "true")
	req.Header.Add(securityTenantHeaderType, "global")

	password, set := c.url.User.Password()
	if set {
		req.SetBasicAuth(c.url.User.Username(), password)
	}

	query := req.URL.Query()
	query.Set("overwrite", "true")
	req.URL.RawQuery = query.Encode()

	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	return &opensearchapi.Response{StatusCode: res.StatusCode, Body: res.Body, Header: res.Header}, nil

}
