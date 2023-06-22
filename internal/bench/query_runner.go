// Copyright 2019 grafana
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package bench

import (
	"context"
	"flag"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/cortexproject/cortex/pkg/util/spanlogger"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_util "github.com/prometheus/common/config"
	"github.com/thanos-io/thanos/pkg/discovery/dns"
	"github.com/thanos-io/thanos/pkg/extprom"
)

type QueryConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Endpoint          string `yaml:"endpoint"`
	BasicAuthUsername string `yaml:"basic_auth_username"`
	BasicAuthPasword  string `yaml:"basic_auth_password"`
}

func (cfg *QueryConfig) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "bench.query.enabled", false, "enable query benchmarking")
	f.StringVar(&cfg.Endpoint, "bench.query.endpoint", "", "Remote query endpoint.")
	f.StringVar(&cfg.BasicAuthUsername, "bench.query.basic-auth-username", "", "Set the basic auth username on remote query requests.")
	f.StringVar(&cfg.BasicAuthPasword, "bench.query.basic-auth-password", "", "Set the basic auth password on remote query requests.")
}

type QueryRunner struct {
	id         string
	tenantName string
	cfg        QueryConfig

	// Do DNS client side load balancing if configured
	dnsProvider *dns.Provider
	addressMtx  sync.Mutex
	addresses   []string
	clientPool  map[string]v1.API

	workload *QueryWorkload

	reg    prometheus.Registerer
	logger log.Logger

	requestDuration *prometheus.HistogramVec
}

func NewQueryRunner(id string, tenantName string, cfg QueryConfig, workload *QueryWorkload, logger log.Logger, reg prometheus.Registerer) (*QueryRunner, error) {
	runner := &QueryRunner{
		id:         id,
		tenantName: tenantName,
		cfg:        cfg,

		workload:   workload,
		clientPool: map[string]v1.API{},
		dnsProvider: dns.NewProvider(
			logger,
			extprom.WrapRegistererWithPrefix("benchtool_query_", reg),
			dns.GolangResolverType,
		),

		logger: logger,
		reg:    reg,
		requestDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "benchtool",
				Name:      "query_request_duration_seconds",
				Buckets:   []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 200},
			},
			[]string{"code", "type"},
		),
	}

	return runner, nil
}

// JitterUp adds random jitter to the duration.
//
// This adds or subtracts time from the duration within a given jitter fraction.
// For example for 10s and jitter 0.1, it will return a time within [9s, 11s])
//
// Reference: https://godoc.org/github.com/grpc-ecosystem/go-grpc-middleware/util/backoffutils
func JitterUp(duration time.Duration, jitter float64) time.Duration {
	multiplier := jitter * (rand.Float64()*2 - 1)
	return time.Duration(float64(duration) * (1 + multiplier))
}

func (q *QueryRunner) Run(ctx context.Context) error {
	go q.ResolveAddrsLoop(ctx)

	queryChan := make(chan Query, 1000)
	for i := 0; i < 100; i++ {
		go q.QueryWorker(queryChan)
	}
	for _, queryReq := range q.workload.Queries {
		// every query has a ticker and a Go loop...
		// not sure if this is a good idea but it should be fine
		go func(req Query) {
			// issue the initial query with a jitter
			firstWait := JitterUp(req.Interval, 0.4)
			time.Sleep(firstWait)
			queryChan <- req

			ticker := time.NewTicker(req.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					queryChan <- req
				case <-ctx.Done():
					return
				}
			}
		}(queryReq)
	}
	for {
		<-ctx.Done()
		close(queryChan)
		return nil

	}
}

func (q *QueryRunner) QueryWorker(queryChan chan Query) {
	for queryReq := range queryChan {
		err := q.ExecuteQuery(context.Background(), queryReq)
		if err != nil {
			level.Warn(q.logger).Log("msg", "unable to execute query", "err", err)
		}
	}
}

type TenantIDRoundTripper struct {
	TenantName string
	Next       http.RoundTripper
}

func (r *TenantIDRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.TenantName != "" {
		req.Header.Set("X-Scope-OrgID", r.TenantName)
	}
	return r.Next.RoundTrip(req)
}

func NewQueryClient(url, tenantName, username, password string) (v1.API, error) {
	apiClient, err := api.NewClient(api.Config{
		Address: url,
		RoundTripper: &TenantIDRoundTripper{
			TenantName: tenantName,
			Next:       config_util.NewBasicAuthRoundTripper(username, config_util.Secret(password), "", api.DefaultRoundTripper),
		},
	})

	if err != nil {
		return nil, err
	}
	return v1.NewAPI(apiClient), nil
}

func (q *QueryRunner) GetRandomAPIClient() (v1.API, error) {
	q.addressMtx.Lock()
	defer q.addressMtx.Unlock()

	if len(q.addresses) == 0 {
		return nil, errors.New("no addresses found")
	}

	randomIndex := rand.Intn(len(q.addresses))
	pick := q.addresses[randomIndex]

	var cli v1.API
	var exists bool
	var err error

	if cli, exists = q.clientPool[pick]; !exists {
		cli, err = NewQueryClient("http://"+pick+"/prometheus", q.tenantName, q.cfg.BasicAuthUsername, q.cfg.BasicAuthPasword)
		if err != nil {
			return nil, err
		}
		q.clientPool[pick] = cli
	}

	return cli, nil
}

func (q *QueryRunner) ExecuteQuery(ctx context.Context, queryReq Query) error {
	spanLog, ctx := spanlogger.New(ctx, "queryRunner.executeQuery")
	defer spanLog.Span.Finish()
	apiClient, err := q.GetRandomAPIClient()
	if err != nil {
		return err
	}

	// Create a timestamp for use when creating the requests and observing latency
	now := time.Now()

	var (
		queryType = "instant"
		status    = "success"
	)
	if queryReq.TimeRange > 0 {
		queryType = "range"
		level.Debug(q.logger).Log("msg", "sending range query", "expr", queryReq.Expr, "range", queryReq.TimeRange)
		r := v1.Range{
			Start: now.Add(-queryReq.TimeRange),
			End:   now,
			Step:  time.Minute,
		}
		_, _, err = apiClient.QueryRange(ctx, queryReq.Expr, r)
	} else {
		level.Debug(q.logger).Log("msg", "sending instant query", "expr", queryReq.Expr)
		_, _, err = apiClient.Query(ctx, queryReq.Expr, now)
	}

	if err != nil {
		status = "failure"
	}

	q.requestDuration.WithLabelValues(status, queryType).Observe(time.Since(now).Seconds())
	return err
}

func (q *QueryRunner) ResolveAddrsLoop(ctx context.Context) {
	err := q.ResolveAddrs()
	if err != nil {
		level.Warn(q.logger).Log("msg", "failed update remote write servers list", "err", err)
	}
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := q.ResolveAddrs()
			if err != nil {
				level.Warn(q.logger).Log("msg", "failed update remote write servers list", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (q *QueryRunner) ResolveAddrs() error {
	// Resolve configured addresses with a reasonable timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// If some of the dns resolution fails, log the error.
	if err := q.dnsProvider.Resolve(ctx, []string{q.cfg.Endpoint}); err != nil {
		level.Error(q.logger).Log("msg", "failed to resolve addresses", "err", err)
	}

	// Fail in case no server address is resolved.
	servers := q.dnsProvider.Addresses()
	if len(servers) == 0 {
		return errors.New("no server address resolved")
	}

	q.addressMtx.Lock()
	q.addresses = servers
	q.addressMtx.Unlock()

	return nil
}
