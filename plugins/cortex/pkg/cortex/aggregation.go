package cortex

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rbac"
	"go.uber.org/zap"
)

type DataFormat string

const (
	// Rule data formatted as a single YAML document containing yaml-encoded
	// []rulefmt.RuleGroup keyed by tenant ID:
	// <tenantID>:
	//   - name: ...
	//     rules: [...]
	NamespaceKeyedYAML DataFormat = "namespace-keyed-yaml"
	// Rule data formatted as JSON containing the prometheus response metadata
	// and a list of rule groups, each of which has a field "file" containing
	// the tenant ID:
	// {"status":"success","data":{"groups":["file":"<tenantID>", ...]}}
	PrometheusRuleGroupsJSON DataFormat = "prometheus-rule-groups-json"
)

type MultiTenantRuleAggregator struct {
	client       managementv1.ManagementClient
	cortexClient *http.Client
	headerCodec  rbac.HeaderCodec
	bufferPool   *sync.Pool
	logger       *zap.SugaredLogger
	format       DataFormat
}

func NewMultiTenantRuleAggregator(
	client managementv1.ManagementClient,
	cortexClient *http.Client,
	headerCodec rbac.HeaderCodec,
	format DataFormat,
) *MultiTenantRuleAggregator {
	pool := &sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, 1024*1024)) // 1MB
		},
	}
	return &MultiTenantRuleAggregator{
		client:       client,
		cortexClient: cortexClient,
		headerCodec:  headerCodec,
		bufferPool:   pool,
		logger:       logger.New().Named("aggregation"),
		format:       format,
	}
}

type rawPromJSONData struct {
	Status string `json:"status"`
	Data   struct {
		Groups []json.RawMessage `json:"groups"`
	} `json:"data"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

func (a *MultiTenantRuleAggregator) Handle(c *gin.Context) {
	ids := rbac.AuthorizedClusterIDs(c)
	a.logger.With(
		"request", c.FullPath(),
	).Debugf("aggregating query over %d tenants", len(ids))

	buf := a.bufferPool.Get().(*bytes.Buffer)
	switch a.format {
	case NamespaceKeyedYAML:
		buf.WriteString("---\n")
	case PrometheusRuleGroupsJSON:
		buf.WriteString(`{"status":"success","data":{"groups":[`)
	}
	// Repeat the request for each cluster ID, sending one ID at a time, and
	// return the aggregated results.
	groupCount := 0
	for _, id := range ids {
		c := c.Copy()
		req := c.Request
		req.Header.Set(a.headerCodec.Key(), a.headerCodec.Encode([]string{id}))

		resp, err := a.cortexClient.Do(req)
		if err != nil {
			a.logger.With(
				"request", c.FullPath(),
				"error", err,
			).Error("error querying cortex")
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		if code := resp.StatusCode; code != http.StatusOK {
			if code == http.StatusNotFound {
				// cortex will report 404 if there are no groups
				continue
			}
			return
		}
		defer resp.Body.Close()
		var bodyReader io.ReadCloser
		switch encoding := c.GetHeader("Content-Encoding"); encoding {
		case "gzip":
			bodyReader, _ = gzip.NewReader(resp.Body)
		case "brotli":
			bodyReader = io.NopCloser(brotli.NewReader(resp.Body))
		case "deflate":
			bodyReader = flate.NewReader(resp.Body)
		case "":
			bodyReader = resp.Body
		default:
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("unsupported Content-Encoding: %s", encoding))
			return
		}

		body, err := io.ReadAll(bodyReader)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		switch a.format {
		case NamespaceKeyedYAML:
			// we can simply concatenate the responses
			buf.Write(body)
		case PrometheusRuleGroupsJSON:
			rawData := rawPromJSONData{}
			if err := json.Unmarshal(body, &rawData); err != nil {
				c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("error parsing cortex response: %s", err))
				return
			}
			if rawData.Status != "success" {
				a.logger.With(
					"request", c.FullPath(),
					"status", rawData.Status,
					"errorType", rawData.ErrorType,
					"error", rawData.Error,
				).Error("error response from prometheus")
				continue
			}
			for _, group := range rawData.Data.Groups {
				raw, _ := group.MarshalJSON()
				if groupCount > 0 {
					buf.WriteString(",")
				}
				groupCount++
				buf.Write(raw)
			}
		}
	}

	switch a.format {
	case NamespaceKeyedYAML:
		c.Data(http.StatusOK, "application/yaml", buf.Bytes())
	case PrometheusRuleGroupsJSON:
		buf.WriteString(`]},"errorType":"","error":""}`)
		c.Data(http.StatusOK, "application/json", buf.Bytes())
	}
}
