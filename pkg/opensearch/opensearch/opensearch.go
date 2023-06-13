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
	ClientOptions
	ISM          api.ISMApi
	Security     api.SecurityAPI
	Indices      api.IndicesAPI
	Ingest       api.IngestAPI
	Tasks        api.TasksAPI
	Cluster      api.ClusterAPI
	Alerting     api.AlertingAPI
	Snapshot     api.SnapshotAPI
	NeuralSearch api.NeuralSearchAPI
}

type ClientConfig struct {
	URLs       []string
	Username   string
	CertReader certs.OpensearchCertReader
}

type ClientOptions struct {
	transport http.RoundTripper
}

type ClientOption func(*ClientOptions)

func (o *ClientOptions) apply(opts ...ClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithTransport(transport http.RoundTripper) ClientOption {
	return func(o *ClientOptions) {
		o.transport = transport
	}
}

func NewClient(cfg ClientConfig, opts ...ClientOption) (*Client, error) {
	urls, err := addrsToURLs(cfg.URLs)
	if err != nil {
		return nil, err
	}

	options := ClientOptions{}
	options.apply(opts...)

	if options.transport == nil {
		if cfg.CertReader == nil {
			return nil, fmt.Errorf("cert reader is required: %w", errors.ErrConfigMissing)
		}
		// Set sane transport timeouts
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext
		transport.TLSHandshakeTimeout = 5 * time.Second

		var usercerts tls.Certificate
		if cfg.Username == "" {
			usercerts, err = cfg.CertReader.GetAdminClientCert()
		} else {
			usercerts, err = cfg.CertReader.GetClientCert(cfg.Username)
		}
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
		options.transport = transport
	}

	client, err := opensearchtransport.New(opensearchtransport.Config{
		URLs:      urls,
		Transport: options.transport,
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
		Indices: api.IndicesAPI{
			Client: client,
		},
		Ingest: api.IngestAPI{
			Client: client,
		},
		Tasks: api.TasksAPI{
			Client: client,
		},
		Cluster: api.ClusterAPI{
			Client: client,
		},
		Alerting: api.AlertingAPI{
			MonitorAPI: api.MonitorAPI{
				Client: client,
			},
			NotificationAPI: api.NotificationAPI{
				Client: client,
			},
			AlertAPI: api.AlertAPI{
				Client: client,
			},
		},
		Snapshot: api.SnapshotAPI{
			Client: client,
		},
		NeuralSearch: api.NeuralSearchAPI{
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
