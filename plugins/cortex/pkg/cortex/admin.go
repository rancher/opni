package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
)

func (p *Plugin) AllUserStats(ctx context.Context, _ *emptypb.Empty) (*cortexadmin.UserIDStatsList, error) {
	client := p.cortexClientSet.Get()
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://%s/distributor/all_user_stats", p.config.Get().Spec.Cortex.Distributor.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.HTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster stats: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			p.logger.With(
				"err", err,
			).Error("failed to close response body")
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get cluster stats: %v", resp.StatusCode)
	}
	var stats []distributor.UserIDStats
	if err = json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode user stats: %w", err)
	}
	statsList := &cortexadmin.UserIDStatsList{
		Items: make([]*cortexadmin.UserIDStats, len(stats)),
	}
	for i, s := range stats {
		statsList.Items[i] = &cortexadmin.UserIDStats{
			UserID:            s.UserID,
			IngestionRate:     s.IngestionRate,
			NumSeries:         s.NumSeries,
			APIIngestionRate:  s.APIIngestionRate,
			RuleIngestionRate: s.RuleIngestionRate,
		}
	}
	return statsList, nil
}

func mapLabels(l *cortexadmin.Label, i int) cortexpb.LabelAdapter {
	return cortexpb.LabelAdapter{
		Name:  l.Name,
		Value: l.Value,
	}
}

func mapSamples(s *cortexadmin.Sample, i int) cortexpb.Sample {
	return cortexpb.Sample{
		TimestampMs: s.TimestampMs,
		Value:       s.Value,
	}
}

func mapExemplars(e *cortexadmin.Exemplar, i int) cortexpb.Exemplar {
	return cortexpb.Exemplar{
		Value:       e.Value,
		TimestampMs: e.TimestampMs,
		Labels:      lo.Map(e.Labels, mapLabels),
	}
}
func mapMetadata(m *cortexadmin.MetricMetadata, i int) *cortexpb.MetricMetadata {
	return &cortexpb.MetricMetadata{
		Type:             cortexpb.MetricMetadata_MetricType(m.Type),
		MetricFamilyName: m.MetricFamilyName,
		Help:             m.Help,
		Unit:             m.Unit,
	}
}

func mapTimeSeries(t *cortexadmin.TimeSeries, i int) cortexpb.PreallocTimeseries {
	return cortexpb.PreallocTimeseries{
		TimeSeries: &cortexpb.TimeSeries{
			Labels:    lo.Map(t.Labels, mapLabels),
			Samples:   lo.Map(t.Samples, mapSamples),
			Exemplars: lo.Map(t.Exemplars, mapExemplars),
		},
	}
}

func (p *Plugin) WriteMetrics(ctx context.Context, in *cortexadmin.WriteRequest) (*cortexadmin.WriteResponse, error) {
	lg := p.logger.With(
		"clusterID", in.ClusterID,
		"seriesCount", len(in.Timeseries),
	)
	if in.ClusterID == "" {
		return nil, status.Error(codes.InvalidArgument, "clusterID is required")
	}
	cortexReq := &cortexpb.WriteRequest{
		Timeseries: lo.Map(in.Timeseries, mapTimeSeries),
		Source:     cortexpb.API,
		Metadata:   lo.Map(in.Metadata, mapMetadata),
	}
	lg.Debug("writing metrics to cortex")
	_, err := p.cortexClientSet.Get().Distributor().Push(outgoingContext(ctx, in), cortexReq)
	if err != nil {
		p.logger.With(zap.Error(err)).Error("failed to write metrics")
		return nil, err
	}
	return &cortexadmin.WriteResponse{}, nil
}

type clusterIDGetter interface {
	GetClusterID() string
}

func outgoingContext(ctx context.Context, in clusterIDGetter) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-scope-orgid", in.GetClusterID(),
	))
}

func (p *Plugin) configureAdminClients() {
	ctx, ca := context.WithTimeout(context.Background(), 10*time.Second)
	defer ca()
	cfg, err := p.config.GetContext(ctx)
	if err != nil {
		p.logger.With(zap.Error(err)).Error("plugin startup failed: config was not loaded")
		os.Exit(1)
	}

	clientset, err := NewClientSet(p.ctx, &cfg.Spec.Cortex, p.cortexTlsConfig.Get())
	if err != nil {
		p.logger.With(zap.Error(err)).Error("plugin startup failed: failed to create cortex client")
		os.Exit(1)
	}
	p.cortexClientSet.Set(clientset)
}

func (p *Plugin) Query(
	ctx context.Context,
	in *cortexadmin.QueryRequest,
) (*cortexadmin.QueryResponse, error) {
	lg := p.logger.With(
		"query", in.Query,
	)
	lg.Debug("handling query")
	client, err := p.cortexClientSet.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/prometheus/api/v1/query", p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Add("query", in.Query)
	req.Body = io.NopCloser(strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode(in.Tenants))
	resp, err := client.HTTP().Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("query failed")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("query failed")
		return nil, fmt.Errorf("query failed: %s", resp.Status)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			lg.With(
				"error", err,
			).Error("failed to close response body")
		}
	}(resp.Body)
	responseBuf := new(bytes.Buffer)
	if _, err := io.Copy(responseBuf, resp.Body); err != nil {
		lg.With(
			"error", err,
		).Error("failed to read response body")
		return nil, err
	}
	return &cortexadmin.QueryResponse{
		Data: responseBuf.Bytes(),
	}, nil
}

func (p *Plugin) QueryRange(
	ctx context.Context,
	in *cortexadmin.QueryRangeRequest,
) (*cortexadmin.QueryResponse, error) {
	lg := p.logger.With(
		"query", in.Query,
	)
	client := p.cortexClientSet.Get()
	values := url.Values{}
	values.Add("query", in.Query)
	values.Add("start", formatTime(in.Start.AsTime()))
	values.Add("end", formatTime(in.End.AsTime()))
	values.Add("step", in.Step.AsDuration().String())

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/prometheus/api/v1/query_range", p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress),
		strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode(in.Tenants))

	lg.Debug(req.URL.RawQuery)
	resp, err := client.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("query failed")
		return nil, fmt.Errorf("query failed: %s", resp.Status)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			lg.With(
				"error", err,
			).Error("failed to close response body")
		}
	}(resp.Body)
	responseBuf := new(bytes.Buffer)
	if _, err := io.Copy(responseBuf, resp.Body); err != nil {
		lg.With(
			"error", err,
		).Error("failed to read response body")
		return nil, err
	}
	return &cortexadmin.QueryResponse{
		Data: responseBuf.Bytes(),
	}, nil
}

func (p *Plugin) GetRule(ctx context.Context,
	in *cortexadmin.RuleRequest,
) (*cortexadmin.QueryResponse, error) {
	lg := p.logger.With(
		"group name", in.GroupName,
	)
	client, err := p.cortexClientSet.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring/%s",
			p.config.Get().Spec.Cortex.Ruler.HTTPAddress, in.GroupName), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.ClusterId}))
	resp, err := client.HTTP().Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("fetch failed")
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		err := status.Error(codes.NotFound, "fetch failed : 404 not found")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("fetch failed")
		return nil, fmt.Errorf("fetch failed: %s", resp.Status)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			lg.With(
				"error", err,
			).Error("failed to close response body")
		}
	}(resp.Body)
	responseBuf := new(bytes.Buffer)
	if _, err := io.Copy(responseBuf, resp.Body); err != nil {
		lg.With(
			"error", err,
		).Error("failed to read response body")
		return nil, err
	}
	return &cortexadmin.QueryResponse{
		Data: responseBuf.Bytes(),
	}, nil
}

func (p *Plugin) ListRules(ctx context.Context, req *cortexadmin.Cluster) (*cortexadmin.QueryResponse, error) {
	lg := p.logger.With(
		"cluster id", req.ClusterId,
	)
	resp, err := listCortexRules(p, lg, ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("list rules failed")
		return nil, fmt.Errorf("list failed: %s", resp.Status)
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return &cortexadmin.QueryResponse{
		Data: body,
	}, nil
}

// LoadRules This method is responsible for Creating and Updating Rules
func (p *Plugin) LoadRules(ctx context.Context,
	in *cortexadmin.PostRuleRequest,
) (*emptypb.Empty, error) {
	lg := p.logger.With(
		"cluster", in.ClusterId,
	)
	client, err := p.cortexClientSet.GetContext(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring", p.config.Get().Spec.Cortex.Ruler.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}

	req.Body = io.NopCloser(strings.NewReader(in.YamlContent))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.ClusterId}))
	resp, err := client.HTTP().Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("loading rules failed")
		return nil, err
	}
	if resp.StatusCode != http.StatusAccepted {
		lg.With(
			"Code", resp.StatusCode,
		).Error("loading rules failed")
		return nil, fmt.Errorf("loading rules failed: %d", resp.StatusCode)
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteRule(
	ctx context.Context,
	in *cortexadmin.RuleRequest,
) (*emptypb.Empty, error) {
	lg := p.logger.With(
		"delete request", in.GroupName,
	)
	client, err := p.cortexClientSet.GetContext(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring/%s",
			p.config.Get().Spec.Cortex.Ruler.HTTPAddress, in.GroupName), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.ClusterId}))
	resp, err := client.HTTP().Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("delete rule group failed")
		return nil, err
	}
	if resp.StatusCode != http.StatusAccepted {
		lg.With(
			"status", resp.Status,
		).Error("delete rule group failed")

		if resp.StatusCode == http.StatusNotFound { // return grpc not found in this case
			err := status.Error(codes.NotFound, fmt.Sprintf("delete rule group failed :`%s`", err))
			return nil, err
		}
		return nil, fmt.Errorf("delete rule group failed: %s", resp.Status)
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetSeriesMetrics(ctx context.Context, request *cortexadmin.SeriesRequest) (*cortexadmin.SeriesInfoList, error) {
	lg := p.logger.With(
		"metric", request.JobId,
	)
	resp, err := enumerateCortexSeries(p, lg, ctx, request)
	if err != nil {
		return nil, err
	}
	set, err := parseCortexEnumerateSeries(resp, lg)
	if err != nil {
		return nil, err
	}
	res := make([]*cortexadmin.SeriesInfo, 0, len(set))
	for uniqueMetricName, _ := range set {
		// fetch metadata & handle empty
		m := &cortexadmin.SeriesMetadata{}
		resp, err := fetchCortexSeriesMetadata(p, lg, ctx, request, uniqueMetricName)
		if err == nil { // parse response, otherwise skip and return empty metadata
			mapVal, err := parseCortexSeriesMetadata(resp, lg, uniqueMetricName)
			if err == nil {
				if metricHelp, ok := mapVal["help"]; ok {
					m.Description = metricHelp.String()
				}
				if metricType, ok := mapVal["type"]; ok {
					m.Type = metricType.String()
				}
				if metricUnit, ok := mapVal["unit"]; ok && metricUnit.String() == "" {
					m.Unit = metricUnit.String()
				}
			}
		}
		res = append(res, &cortexadmin.SeriesInfo{
			SeriesName: uniqueMetricName,
			Metadata:   m,
		})
	}

	return &cortexadmin.SeriesInfoList{
		Items: res,
	}, nil
}

func (p *Plugin) GetMetricLabelSets(ctx context.Context, request *cortexadmin.LabelRequest) (*cortexadmin.MetricLabels, error) {
	lg := p.logger.With(
		"service", request.JobId,
		"metric", request.MetricName,
	)
	resp, err := enumerateCortexSeries(p, lg, ctx, &cortexadmin.SeriesRequest{
		Tenant: request.Tenant,
		JobId:  request.JobId,
	})
	if err != nil {
		return nil, err
	}
	labelSets, err := parseCortexLabelsOnSeriesJob(resp, request.MetricName, request.JobId, lg)
	if err != nil {
		return nil, err
	}
	resultSets := []*cortexadmin.LabelSet{}
	for labelName, labelValues := range labelSets {
		item := &cortexadmin.LabelSet{
			Name:  labelName,
			Items: []string{},
		}
		for labelVal, _ := range labelValues {
			item.Items = append(item.Items, labelVal)
		}
		resultSets = append(resultSets, item)
	}
	return &cortexadmin.MetricLabels{
		Items: resultSets,
	}, nil
}

func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.Unix())+float64(t.Nanosecond())/1e9, 'f', -1, 64)
}

// subset of response types from cortex pkg/ring/http.go
type ingesterDesc struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Address string `json:"address"`
}

type httpResponse struct {
	Ingesters []ingesterDesc `json:"shards"`
}

func (p *Plugin) FlushBlocks(
	ctx context.Context,
	in *emptypb.Empty,
) (*emptypb.Empty, error) {
	// look up all healthy ingesters
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://%s/ingester/ring", p.config.Get().Spec.Cortex.Distributor.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	client, err := p.cortexClientSet.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	resp, err := client.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get ingester ring: %s", resp.Status)
	}
	var ring httpResponse
	body, _ := io.ReadAll(resp.Body)
	err = resp.Body.Close()
	if err != nil {
		p.logger.Error("failed to close response body")
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&ring); err != nil {
		return nil, err
	}

	// flush all active ingesters
	wg := errgroup.Group{}
	for _, ingester := range ring.Ingesters {
		lg := p.logger.With(
			"id", ingester.ID,
		)
		if ingester.State != "ACTIVE" {
			lg.Warn("not flushing inactive ingester")
			continue
		}

		// the ring only shows the grpc port, so switch it with the http port
		// from the cortex config
		host, _, err := net.SplitHostPort(ingester.Address)
		if err != nil {
			return nil, err
		}
		// set the hostname from the cortex config as the tls server name, since
		// the server's cert won't have a san for its ip in the pod cidr
		hostSan, port, err := net.SplitHostPort(p.config.Get().Spec.Cortex.Distributor.HTTPAddress)
		if err != nil {
			return nil, err
		}
		address := fmt.Sprintf("%s:%s", host, port)

		wg.Go(func() error {
			// adding ?wait=true will cause this request to block
			values := url.Values{}
			values.Add("wait", "true")
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				fmt.Sprintf("https://%s/ingester/flush", address), strings.NewReader(values.Encode()))
			if err != nil {
				return err
			}
			config := p.cortexTlsConfig.Get().Clone()
			config.ServerName = hostSan
			clientWithNameOverride := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: config,
				},
			}
			resp, err := clientWithNameOverride.Do(req)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("failed to flush ingester")
				return err
			}
			if resp.StatusCode != http.StatusNoContent {
				body, _ := io.ReadAll(resp.Body)
				err := resp.Body.Close()
				if err != nil {
					lg.Error(
						"failed to close response body",
					)
				}
				lg.With(
					"code", resp.StatusCode,
					"error", string(body),
				).Errorf("failed to flush ingester")
			}

			lg.Info("flushed ingester successfully")
			return nil
		})
	}
	err = wg.Wait()
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
