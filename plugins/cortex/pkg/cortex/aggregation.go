package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/rbac"
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
	client      management.ManagementClient
	forwarder   fiber.Handler
	headerCodec rbac.HeaderCodec
	bufferPool  *sync.Pool
	logger      *zap.SugaredLogger
	format      DataFormat
}

func NewMultiTenantRuleAggregator(
	client management.ManagementClient,
	forwarder fiber.Handler,
	headerCodec rbac.HeaderCodec,
	format DataFormat,
) *MultiTenantRuleAggregator {
	pool := &sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, 1024*1024)) // 1MB
		},
	}
	return &MultiTenantRuleAggregator{
		client:      client,
		forwarder:   forwarder,
		headerCodec: headerCodec,
		bufferPool:  pool,
		logger:      logger.New().Named("aggregation"),
		format:      format,
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

func (a *MultiTenantRuleAggregator) Handle(c *fiber.Ctx) error {
	ids := rbac.AuthorizedClusterIDs(c)
	a.logger.With(
		"request", c.Path(),
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
		req := c.Request()
		req.Header.Set(a.headerCodec.Key(), a.headerCodec.Encode([]string{id}))
		err := a.forwarder(c)
		if err != nil {
			return err
		}
		resp := c.Response()
		if code := resp.StatusCode(); code != fiber.StatusOK {
			if code == fiber.StatusNotFound {
				// cortex will report 404 if there are no groups
				continue
			}
			return nil
		}
		var body []byte
		switch encoding := c.GetRespHeader("Content-Encoding", ""); encoding {
		case "gzip":
			body, err = resp.BodyGunzip()
			if err != nil {
				return err
			}
		case "brotli":
			body, err = resp.BodyUnbrotli()
			if err != nil {
				return err
			}
		case "deflate":
			body, err = resp.BodyInflate()
			if err != nil {
				return err
			}
		case "":
			body = resp.Body()
		default:
			return fmt.Errorf("unsupported Content-Encoding: %s", encoding)
		}

		switch a.format {
		case NamespaceKeyedYAML:
			// we can simply concatenate the responses
			buf.Write(body)
		case PrometheusRuleGroupsJSON:
			rawData := rawPromJSONData{}
			if err := json.Unmarshal(body, &rawData); err != nil {
				return err
			}
			if rawData.Status != "success" {
				a.logger.With(
					"request", c.Path(),
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
		resp.Reset()
	}

	switch a.format {
	case NamespaceKeyedYAML:
		c.Set("Content-Type", "application/yaml")
	case PrometheusRuleGroupsJSON:
		c.Set("Content-Type", "application/json")
		buf.WriteString(`]},"errorType":"","error":""}`)
	}
	c.Response().SetBody(buf.Bytes())
	c.Status(fiber.StatusOK)
	return nil
}
