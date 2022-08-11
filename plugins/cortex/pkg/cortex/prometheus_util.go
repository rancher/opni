package cortex

import (
	"context"
	"fmt"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
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
	client, err := p.cortexHttpClient.GetContext(ctx)
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
	resp, err := client.Do(req)
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
	val, err := unmarshal.UnmarshallPrometheusWebResponse(resp, lg)
	if err != nil {
		return nil, err
	}
	set = make(map[string]struct{})
	// must convert to slice
	interfaceSlice, err := unmarshal.InterfaceToInterfaceSlice(val.Data)
	if err != nil {
		return nil, err
	}
	// must convert each to a map[string]interface{}
	for _, i := range interfaceSlice {
		mapStruct, err := unmarshal.InterfaceToMap(i)
		if err != nil {
			continue
		}
		if v, ok := mapStruct["__name__"]; ok {
			set[fmt.Sprintf("%v", v)] = struct{}{}
		}
	}
	return set, nil
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

func parseCortexSeriesMetadata(resp *http.Response, lg *zap.SugaredLogger, metricName string) (map[string]interface{}, error) {
	val, err := unmarshal.UnmarshallPrometheusWebResponse(resp, lg)
	if err != nil {
		return nil, err
	}
	if val.Data == nil { //FIXME currently cortex always returns nil here & its not clear why
		return nil, fmt.Errorf("no metadata in response")
	}
	// otherwise it is a map -> list -> map[string]interface{}
	valMap, err := unmarshal.InterfaceToMap(val.Data)
	if err != nil {
		return nil, err
	}
	if _, ok := valMap[metricName]; !ok {
		return nil, fmt.Errorf(fmt.Sprintf("no metadata for metric %s in response", metricName))
	}
	// technically each metric name can have multiple metadata if you set it up in a stupid way
	bestResult, err := unmarshal.InterfaceToInterfaceSlice(valMap[metricName])
	if err != nil {
		return nil, err
	}
	// first result is not necessarily the best result
	res, err := unmarshal.InterfaceToMap(bestResult[0])
	return res, err
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
	allLabelsStruct, err := unmarshal.UnmarshallPrometheusWebResponse(resp, p.logger)
	if err != nil {
		return nil, err
	}
	labelNames, err := unmarshal.InterfaceToStringSlice(allLabelsStruct.Data)
	if err != nil {
		return nil, err
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
