package cortex

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func proxyCortexToPrometheus(
	p *Plugin,
	lg *zap.SugaredLogger,
	ctx context.Context,
	tenant string,
	method string,
	url string,
	values url.Values,
	body io.Reader,
) (*http.Response, error) {
	client, err := p.cortexClientSet.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		url,
		body,
	)
	if values != nil {
		req.URL.RawQuery = values.Encode()
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{tenant}))
	resp, err := client.HTTP().Do(req)
	if err != nil {
		lg.With(
			"request", url,
		).Error(fmt.Sprintf("failed with %v", err))
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"request", url,
		).Error("request failed with %s", resp.Status)
		return nil, fmt.Errorf("request failed with: %s", resp.Status)
	}
	return resp, nil
}

// returns duplicate metric names, since labels uniquely identify series but not metrics
func enumerateCortexSeries(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, request *cortexadmin.SeriesRequest) (*http.Response, error) {
	values := url.Values{}
	values.Add("match[]", fmt.Sprintf("{job=\"%s\"}", request.JobId))
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/series?",
		p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := proxyCortexToPrometheus(p, lg, ctx, request.Tenant, "GET", reqUrl, values, nil)
	return resp, err
}

func parseCortexEnumerateSeries(resp *http.Response, lg *zap.SugaredLogger) (set map[string]struct{}, err error) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if !gjson.Valid(string(b)) {
		return nil, fmt.Errorf("invalid json in response")
	}
	set = make(map[string]struct{})
	result := gjson.Get(string(b), "data.#.__name__")
	if !result.Exists() {
		return nil, fmt.Errorf("Empty series response from cortex")
	}
	for _, name := range result.Array() {
		set[name.String()] = struct{}{}
	}
	return set, nil
}

// parseCortexLabelsOnSeriesJob parses the cortex response and returns a map labelNames -> set of labelValues
func parseCortexLabelsOnSeriesJob(
	resp *http.Response,
	metricName string,
	jobName string,
	lg *zap.SugaredLogger,
) (map[string]map[string]struct{}, error) {
	labelSets := map[string]map[string]struct{}{} // labelName -> set of labelValues
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if !gjson.Valid(string(b)) {
		return nil, fmt.Errorf("invalid json in response")
	}
	//labelSets := make(map[string]map[string]struct{})
	result := gjson.Get(string(b), "data")
	if !result.Exists() {
		return nil, fmt.Errorf("no data in cortex response")
	}
	for _, val := range result.Array() {
		valToMap := val.Map()
		if valToMap["__name__"].String() != metricName || valToMap["job"].String() != jobName {
			continue
		}
		for k, v := range valToMap {
			if (k == "__name__" && valToMap[k].String() == metricName) || (k == "job" && valToMap[k].String() == jobName) {
				continue
			}
			if _, ok := labelSets[k]; !ok {
				labelSets[k] = make(map[string]struct{})
			}
			labelSets[k][v.String()] = struct{}{}
		}
	}

	return labelSets, nil
}

func fetchCortexSeriesMetadata(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, request *cortexadmin.SeriesRequest, metricName string) (*http.Response, error) {
	values := url.Values{}
	values.Add("metric", metricName)
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/metadata?",
		p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := proxyCortexToPrometheus(p, lg, ctx, request.Tenant, "GET", reqUrl, values, nil)
	return resp, err
}

func parseCortexSeriesMetadata(resp *http.Response, lg *zap.SugaredLogger, metricName string) (map[string]gjson.Result, error) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if !gjson.Valid(string(b)) {
		return nil, fmt.Errorf("invalid json in response")
	}
	result := gjson.Get(string(b), fmt.Sprintf("data.%s)", metricName))
	if !result.Exists() {
		return nil, fmt.Errorf("no metadata in cortex response")
	}
	metadata := result.Array()[0].Map()
	return metadata, err
}

func getCortexMetricLabels(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, request *cortexadmin.LabelRequest) (*http.Response, error) {
	values := url.Values{}
	values.Add("match[]", fmt.Sprintf("%s{job=\"%s\"}", request.MetricName, request.JobId)) // encode the input metric
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/labels",
		p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := proxyCortexToPrometheus(p, lg, ctx, request.Tenant, "GET", reqUrl, values, nil)
	return resp, err
}

func parseCortexMetricLabels(p *Plugin, resp *http.Response) ([]string, error) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	labelNames := []string{}
	result := gjson.Get(string(b), "data")
	for _, name := range result.Array() {
		if name.String() == "__name__" || name.String() == "job" {
			continue
		}
		labelNames = append(labelNames, name.String())
	}
	return labelNames, nil
}

func getCortexLabelValues(p *Plugin, ctx context.Context, request *cortexadmin.LabelRequest, labelName string) (*http.Response, error) {
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/label/%s/values",
		p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
		labelName,
	)
	resp, err := proxyCortexToPrometheus(p, p.logger, ctx, request.Tenant, "GET", reqUrl, nil, nil)
	return resp, err
}

func listCortexRules(p *Plugin, lg *zap.SugaredLogger, ctx context.Context, request *cortexadmin.Cluster) (*http.Response, error) {
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/rules",
		p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := proxyCortexToPrometheus(p, lg, ctx, request.ClusterId, "GET", reqUrl, nil, nil)
	return resp, err
}
