package opensearch

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2/opensearchtransport"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"github.com/rancher/opni/pkg/opensearch/opensearch/api"
	"github.com/rancher/opni/pkg/opensearch/opensearch/errors"
)

type Client struct {
	ISM      api.ISMApi
	Security api.SecurityAPI
}

type ClientConfig struct {
	URLs       []string
	Username   string
	CertReader certs.OpensearchCertReader
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.CertReader == nil {
		return nil, fmt.Errorf("cert reader is required: %w", errors.ErrConfigMissing)
	}
	urls, err := addrsToURLs(cfg.URLs)
	if err != nil {
		return nil, err
	}
	// Set sane transport timeouts
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout: 5 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second

	usercerts, err := cfg.CertReader.GetClientCertificate(cfg.Username)
	if err != nil {
		return nil, err
	}

	cacerts, err := cfg.CertReader.GetHTTPRootCAs()
	if err != nil {
		return nil, err
	}

	transport.TLSClientConfig = &tls.Config{
		Certificates: []tls.Certificate{
			usercerts,
		},
		RootCAs: cacerts,
	}

	client, err := opensearchtransport.New(opensearchtransport.Config{
		URLs:      urls,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		ISM: api.ISMApi{
			Client: client,
		},
		Security: api.SecurityAPI{
			Client: client,
		},
	}, nil
}

func addrsToURLs(addrs []string) ([]*url.URL, error) {
	var urls []*url.URL
	for _, addr := range addrs {
		u, err := url.Parse(strings.TrimRight(addr, "/"))
		if err != nil {
			return nil, fmt.Errorf("cannot parse url: %v", err)
		}

		urls = append(urls, u)
	}
	return urls, nil
}
