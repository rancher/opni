package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// this is 'github.com/cortexproject/cortex/pkg/distributor.UserIDStats'
// but importing it causes impossible dependency errors
type CortexUserIDStats struct {
	UserID            string  `json:"userID"`
	IngestionRate     float64 `json:"ingestionRate"`
	NumSeries         uint64  `json:"numSeries"`
	APIIngestionRate  float64 `json:"APIIngestionRate"`
	RuleIngestionRate float64 `json:"RuleIngestionRate"`
}

func (p *Plugin) AllUserStats(ctx context.Context, _ *emptypb.Empty) (*cortexadmin.UserIDStatsList, error) {
	client := p.cortexHttpClient.Get()
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://%s/distributor/all_user_stats", p.config.Get().Spec.Cortex.Distributor.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster stats: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get cluster stats: %v", resp.StatusCode)
	}
	var stats []CortexUserIDStats
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
	_, err := p.distributorClient.Get().Push(outgoingContext(ctx, in), cortexReq)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to write metrics")
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
		p.logger.With("err", err).Error("plugin startup failed: config was not loaded")
		os.Exit(1)
	}

	{
		cc, err := grpc.DialContext(p.ctx, cfg.Spec.Cortex.Distributor.GRPCAddress,
			grpc.WithTransportCredentials(credentials.NewTLS(p.cortexTlsConfig.Get())),
			grpc.WithChainStreamInterceptor(otelgrpc.StreamClientInterceptor()),
			grpc.WithChainUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		)
		if err != nil {
			p.logger.With(
				"err", err,
			).Error("Failed to dial distributor")
			os.Exit(1)
		}
		p.distributorClient.Set(distributorpb.NewDistributorClient(cc))
	}
	{
		httpClient := &http.Client{
			Transport: otelhttp.NewTransport(&http.Transport{
				TLSClientConfig: p.cortexTlsConfig.Get(),
			}),
		}
		p.cortexHttpClient.Set(httpClient)
	}
}

func (p *Plugin) Query(
	ctx context.Context,
	in *cortexadmin.QueryRequest,
) (*cortexadmin.QueryResponse, error) {
	lg := p.logger.With(
		"query", in.Query,
	)
	lg.Debug("handling query")
	client, err := p.cortexHttpClient.GetContext(ctx)
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
	resp, err := client.Do(req)
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
	defer resp.Body.Close()
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
	client := p.cortexHttpClient.Get()
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("query failed")
		return nil, fmt.Errorf("query failed: %s", resp.Status)
	}
	defer resp.Body.Close()
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
	client, err := p.cortexHttpClient.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring/%s",
			p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress, in.GroupName), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.Tenant}))
	resp, err := client.Do(req)
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
	defer resp.Body.Close()
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

// This method is responsible for Creating and Updating Rules
func (p *Plugin) LoadRules(ctx context.Context,
	in *cortexadmin.YamlRequest,
) (*emptypb.Empty, error) {
	lg := p.logger.With(
		"yaml", in.Yaml,
	)
	client, err := p.cortexHttpClient.GetContext(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring", p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Add("yaml", in.Yaml)
	req.Body = io.NopCloser(strings.NewReader(in.Yaml))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.Tenant}))
	resp, err := client.Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("loading rules failed")
		return nil, err
	}
	if resp.StatusCode != http.StatusAccepted {
		lg.With(
			"status", resp.Status,
		).Error("loading rules failed")
		return nil, fmt.Errorf("loading rules failed: %s", resp.Status)
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteRule(
	ctx context.Context, in *cortexadmin.RuleRequest,
) (*emptypb.Empty, error) {
	lg := p.logger.With(
		"delete request", in.GroupName,
	)
	client, err := p.cortexHttpClient.GetContext(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring/%s",
			p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress, in.GroupName), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.Tenant}))
	resp, err := client.Do(req)
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
		return nil, fmt.Errorf("delete rule group failed: %s", resp.Status)
	}
	return &emptypb.Empty{}, nil
}

func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.Unix())+float64(t.Nanosecond())/1e9, 'f', -1, 64)
}
