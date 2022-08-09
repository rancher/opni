package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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
	client, err := p.cortexHttpClient.GetContext(ctx)
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

// LoadRules This method is responsible for Creating and Updating Rules
func (p *Plugin) LoadRules(ctx context.Context,
	in *cortexadmin.PostRuleRequest,
) (*emptypb.Empty, error) {
	lg := p.logger.With(
		"cluster", in.ClusterId,
	)
	client, err := p.cortexHttpClient.GetContext(ctx)

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
	resp, err := client.Do(req)
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
	client, err := p.cortexHttpClient.GetContext(ctx)

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

		if resp.StatusCode == http.StatusNotFound { // return grpc not found in this case
			err := status.Error(codes.NotFound, fmt.Sprintf("delete rule group failed :`%s`", err))
			return nil, err
		}
		return nil, fmt.Errorf("delete rule group failed: %s", resp.Status)
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetSeriesMetadata(ctx context.Context, request *cortexadmin.SeriesRequest) (*cortexadmin.MetricMetadata, error) {
	lg := p.logger.With(
		"metric", request.MetricName,
	)
	client, err := p.cortexHttpClient.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf(
			"https://%s/prometheus/api/v1/metadata",
			p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
		),
		nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{request.Tenant}))
	resp, err := client.Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("fetch series metric metadata failed")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("fetch series metric metadata failed")
		return nil, fmt.Errorf("fetch series metric metadata failed: %s", resp.Status)
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
	val := string(responseBuf.Bytes())
	lg.Debug(val)
	return &cortexadmin.MetricMetadata{}, nil
}

func (p *Plugin) GetMetricLabels(ctx context.Context, request *cortexadmin.SeriesRequest) (*cortexadmin.MetricLabels, error) {
	lg := p.logger.With(
		"metric", request.MetricName,
	)
	client, err := p.cortexHttpClient.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf(
			"https://%s/prometheus/api/v1/labels",
			p.config.Get().Spec.Cortex.QueryFrontend.HTTPAddress,
		),
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{request.Tenant}))
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		lg.With(
			"error", err,
		).Error("fetch series metric metadata failed")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("fetch series metric metadata failed")
		return nil, fmt.Errorf("fetch series metric metadata failed: %s", resp.Status)
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
	val := string(responseBuf.Bytes())
	lg.Debug(val)
	return &cortexadmin.MetricLabels{Labels: []string{"none"}}, nil
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

	client, err := p.cortexHttpClient.GetContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cortex http client: %w", err)
	}
	resp, err := client.Do(req)
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
