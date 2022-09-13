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
	"strconv"
	"strings"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/distributor"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
)

type CortexAdminServer struct {
	cortexadmin.UnsafeCortexAdminServer
	CortexAdminServerConfig
	metricsutil.Initializer
}

type CortexAdminServerConfig struct {
	CortexClientSet ClientSet                  `validate:"required"`
	Config          *v1beta1.GatewayConfigSpec `validate:"required"`
	Logger          *zap.SugaredLogger         `validate:"required"`
}

func (p *CortexAdminServer) Initialize(conf CortexAdminServerConfig) {
	p.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		p.CortexAdminServerConfig = conf
	})
}

var _ cortexadmin.CortexAdminServer = (*CortexAdminServer)(nil)

func (p *CortexAdminServer) AllUserStats(ctx context.Context, _ *emptypb.Empty) (*cortexadmin.UserIDStatsList, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	client := p.CortexClientSet
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://%s/distributor/all_user_stats", p.Config.Cortex.Distributor.HTTPAddress), nil)
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
			p.Logger.With(
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

func (p *CortexAdminServer) WriteMetrics(ctx context.Context, in *cortexadmin.WriteRequest) (*cortexadmin.WriteResponse, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
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
	_, err := p.CortexClientSet.Distributor().Push(outgoingContext(ctx, in), cortexReq)
	if err != nil {
		p.Logger.With(zap.Error(err)).Error("failed to write metrics")
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

func (p *CortexAdminServer) Query(
	ctx context.Context,
	in *cortexadmin.QueryRequest,
) (*cortexadmin.QueryResponse, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
		"query", in.Query,
	)
	lg.Debug("handling query")

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/prometheus/api/v1/query", p.Config.Cortex.QueryFrontend.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Add("query", in.Query)
	req.Body = io.NopCloser(strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode(in.Tenants))
	resp, err := p.CortexClientSet.HTTP().Do(req)
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

func (p *CortexAdminServer) QueryRange(
	ctx context.Context,
	in *cortexadmin.QueryRangeRequest,
) (*cortexadmin.QueryResponse, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
		"query", in.Query,
	)
	client := p.CortexClientSet
	values := url.Values{}
	values.Add("query", in.Query)
	values.Add("start", formatTime(in.Start.AsTime()))
	values.Add("end", formatTime(in.End.AsTime()))
	values.Add("step", in.Step.AsDuration().String())

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/prometheus/api/v1/query_range", p.Config.Cortex.QueryFrontend.HTTPAddress),
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

func (p *CortexAdminServer) GetRule(ctx context.Context,
	in *cortexadmin.RuleRequest,
) (*cortexadmin.QueryResponse, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
		"group name", in.GroupName,
	)

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring/%s",
			p.Config.Cortex.Ruler.HTTPAddress, in.GroupName), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.ClusterId}))
	resp, err := p.CortexClientSet.HTTP().Do(req)
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

func (p *CortexAdminServer) ListRules(ctx context.Context, req *cortexadmin.Cluster) (*cortexadmin.QueryResponse, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
		"cluster id", req.ClusterId,
	)
	resp, err := p.listCortexRules(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		lg.With(
			"status", resp.Status,
		).Error("list rules failed")
		return nil, fmt.Errorf("list failed: %s", resp.Status)
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
func (p *CortexAdminServer) LoadRules(ctx context.Context,
	in *cortexadmin.PostRuleRequest,
) (*emptypb.Empty, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
		"cluster", in.ClusterId,
	)

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring", p.Config.Cortex.Ruler.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}

	req.Body = io.NopCloser(strings.NewReader(in.YamlContent))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.ClusterId}))
	resp, err := p.CortexClientSet.HTTP().Do(req)
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

func (p *CortexAdminServer) DeleteRule(
	ctx context.Context,
	in *cortexadmin.RuleRequest,
) (*emptypb.Empty, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	lg := p.Logger.With(
		"delete request", in.GroupName,
	)

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("https://%s/api/v1/rules/monitoring/%s",
			p.Config.Cortex.Ruler.HTTPAddress, in.GroupName), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(orgIDCodec.Key(), orgIDCodec.Encode([]string{in.ClusterId}))
	resp, err := p.CortexClientSet.HTTP().Do(req)
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

func (p *CortexAdminServer) GetSeriesMetrics(ctx context.Context, request *cortexadmin.SeriesRequest) (*cortexadmin.SeriesInfoList, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	resp, err := p.enumerateCortexSeries(ctx, request)
	if err != nil {
		return nil, err
	}
	set, err := p.parseCortexEnumerateSeries(resp)
	if err != nil {
		return nil, err
	}
	res := make([]*cortexadmin.SeriesInfo, 0, len(set))
	for uniqueMetricName, _ := range set {
		// fetch metadata & handle empty
		m := &cortexadmin.SeriesMetadata{}
		resp, err := p.fetchCortexSeriesMetadata(ctx, request, uniqueMetricName)
		if err == nil { // parse response, otherwise skip and return empty metadata
			mapVal, err := p.parseCortexSeriesMetadata(resp, uniqueMetricName)
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

func (p *CortexAdminServer) GetMetricLabelSets(ctx context.Context, request *cortexadmin.LabelRequest) (*cortexadmin.MetricLabels, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	resp, err := p.enumerateCortexSeries(ctx, &cortexadmin.SeriesRequest{
		Tenant: request.Tenant,
		JobId:  request.JobId,
	})
	if err != nil {
		return nil, err
	}
	labelSets, err := p.parseCortexLabelsOnSeriesJob(resp, request.MetricName, request.JobId)
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

func (p *CortexAdminServer) FlushBlocks(
	ctx context.Context,
	in *emptypb.Empty,
) (*emptypb.Empty, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	// look up all healthy ingesters
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://%s/ingester/ring", p.Config.Cortex.Distributor.HTTPAddress), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := p.CortexClientSet.HTTP().Do(req)
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
		p.Logger.Error("failed to close response body")
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&ring); err != nil {
		return nil, err
	}

	// flush all active ingesters
	wg := errgroup.Group{}
	for _, ingester := range ring.Ingesters {
		lg := p.Logger.With(
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
		hostSan, port, err := net.SplitHostPort(p.Config.Cortex.Distributor.HTTPAddress)
		if err != nil {
			return nil, err
		}
		address := fmt.Sprintf("%s:%s", host, port)

		httpClient := p.CortexClientSet.HTTP(WithServerNameOverride(hostSan))
		wg.Go(func() error {
			// adding ?wait=true will cause this request to block
			values := url.Values{}
			values.Add("wait", "true")
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				fmt.Sprintf("https://%s/ingester/flush", address), strings.NewReader(values.Encode()))
			if err != nil {
				return err
			}
			resp, err := httpClient.Do(req)
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

func (p *CortexAdminServer) GetCortexStatus(ctx context.Context, _ *emptypb.Empty) (*cortexadmin.CortexStatus, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	stat := &cortexadmin.CortexStatus{}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() (err error) {
		stat.Distributor, err = p.CortexClientSet.Distributor().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Ingester, err = p.CortexClientSet.Ingester().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Ruler, err = p.CortexClientSet.Ruler().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Purger, err = p.CortexClientSet.Purger().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Compactor, err = p.CortexClientSet.Compactor().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.StoreGateway, err = p.CortexClientSet.StoreGateway().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.QueryFrontend, err = p.CortexClientSet.QueryFrontend().Status(ctx)
		return
	})
	eg.Go(func() (err error) {
		stat.Querier, err = p.CortexClientSet.Querier().Status(ctx)
		return
	})

	if err := eg.Wait(); err != nil {
		p.Logger.With(
			zap.Error(err),
		).Error("failed to get cluster status")
		return nil, err
	}
	return stat, nil
}

func (p *CortexAdminServer) GetCortexConfig(ctx context.Context, req *cortexadmin.ConfigRequest) (*cortexadmin.ConfigResponse, error) {
	if !p.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}
	resp := &cortexadmin.ConfigResponse{
		ConfigYaml: make([]string, len(req.ConfigModes)),
	}

	eg, ctx := errgroup.WithContext(ctx)

	for i, mode := range req.ConfigModes {
		i := i
		mode := mode
		eg.Go(func() (err error) {
			resp.ConfigYaml[i], err = p.CortexClientSet.Distributor().Config(ctx, ConfigMode(mode))
			return
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *CortexAdminServer) proxyCortexToPrometheus(
	ctx context.Context,
	tenant string,
	method string,
	url string,
	values url.Values,
	body io.Reader,
) (*http.Response, error) {
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
	resp, err := p.CortexClientSet.HTTP().Do(req)
	if err != nil {
		p.Logger.With(
			"request", url,
		).Error(fmt.Sprintf("failed with %v", err))
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		p.Logger.With(
			"request", url,
		).Error("request failed with %s", resp.Status)
		return nil, fmt.Errorf("request failed with: %s", resp.Status)
	}
	return resp, nil
}

// returns duplicate metric names, since labels uniquely identify series but not metrics
func (p *CortexAdminServer) enumerateCortexSeries(ctx context.Context, request *cortexadmin.SeriesRequest) (*http.Response, error) {
	values := url.Values{}
	values.Add("match[]", fmt.Sprintf("{job=\"%s\"}", request.JobId))
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/series?",
		p.Config.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := p.proxyCortexToPrometheus(ctx, request.Tenant, "GET", reqUrl, values, nil)
	return resp, err
}

func (p *CortexAdminServer) parseCortexEnumerateSeries(resp *http.Response) (set map[string]struct{}, err error) {
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
func (p *CortexAdminServer) parseCortexLabelsOnSeriesJob(
	resp *http.Response,
	metricName string,
	jobName string,
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

func (p *CortexAdminServer) fetchCortexSeriesMetadata(ctx context.Context, request *cortexadmin.SeriesRequest, metricName string) (*http.Response, error) {
	values := url.Values{}
	values.Add("metric", metricName)
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/metadata?",
		p.Config.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := p.proxyCortexToPrometheus(ctx, request.Tenant, "GET", reqUrl, values, nil)
	return resp, err
}

func (p *CortexAdminServer) parseCortexSeriesMetadata(resp *http.Response, metricName string) (map[string]gjson.Result, error) {
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

func (p *CortexAdminServer) getCortexMetricLabels(ctx context.Context, request *cortexadmin.LabelRequest) (*http.Response, error) {
	values := url.Values{}
	values.Add("match[]", fmt.Sprintf("%s{job=\"%s\"}", request.MetricName, request.JobId)) // encode the input metric
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/labels",
		p.Config.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := p.proxyCortexToPrometheus(ctx, request.Tenant, "GET", reqUrl, values, nil)
	return resp, err
}

func (p *CortexAdminServer) parseCortexMetricLabels(resp *http.Response) ([]string, error) {
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

func (p *CortexAdminServer) getCortexLabelValues(ctx context.Context, request *cortexadmin.LabelRequest, labelName string) (*http.Response, error) {
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/label/%s/values",
		p.Config.Cortex.QueryFrontend.HTTPAddress,
		labelName,
	)
	resp, err := p.proxyCortexToPrometheus(ctx, request.Tenant, "GET", reqUrl, nil, nil)
	return resp, err
}

func (p *CortexAdminServer) listCortexRules(ctx context.Context, request *cortexadmin.Cluster) (*http.Response, error) {
	reqUrl := fmt.Sprintf(
		"https://%s/prometheus/api/v1/rules",
		p.Config.Cortex.QueryFrontend.HTTPAddress,
	)
	resp, err := p.proxyCortexToPrometheus(ctx, request.ClusterId, "GET", reqUrl, nil, nil)
	return resp, err
}
