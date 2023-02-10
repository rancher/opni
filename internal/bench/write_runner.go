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
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/thanos-io/thanos/pkg/discovery/dns"
	"github.com/thanos-io/thanos/pkg/extprom"
)

type WriteBenchConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Endpoint          string `yaml:"endpoint"`
	BasicAuthUsername string `yaml:"basic_auth_username"`
	BasicAuthPasword  string `yaml:"basic_auth_password"`
	ProxyURL          string `yaml:"proxy_url"`
}

func (cfg *WriteBenchConfig) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "bench.write.enabled", false, "enable write benchmarking")
	f.StringVar(&cfg.Endpoint, "bench.write.endpoint", "", "Remote write endpoint.")
	f.StringVar(&cfg.BasicAuthUsername, "bench.write.basic-auth-username", "", "Set the basic auth username on remote write requests.")
	f.StringVar(&cfg.BasicAuthPasword, "bench.write.basic-auth-password", "", "Set the basic auth password on remote write requests.")
	f.StringVar(&cfg.ProxyURL, "bench.write.proxy-url", "", "Set the HTTP proxy URL to use on remote write requests.")
}

type WriteBenchmarkRunner struct {
	id         string
	tenantName string
	cfg        WriteBenchConfig

	// Do DNS client side load balancing if configured
	remoteMtx  sync.Mutex
	addresses  []string
	clientPool map[string]*WriteClient

	dnsProvider *dns.Provider

	workload *WriteWorkload

	reg    prometheus.Registerer
	logger log.Logger

	requestDuration *prometheus.HistogramVec
}

func NewWriteBenchmarkRunner(id string, tenantName string, cfg WriteBenchConfig, workload *WriteWorkload, logger log.Logger, reg prometheus.Registerer) (*WriteBenchmarkRunner, error) {
	writeBench := &WriteBenchmarkRunner{
		id:         id,
		tenantName: tenantName,
		cfg:        cfg,

		workload: workload,
		dnsProvider: dns.NewProvider(
			logger,
			extprom.WrapRegistererWithPrefix("benchtool_write_", reg),
			dns.GolangResolverType,
		),
		clientPool: map[string]*WriteClient{},
		logger:     logger,
		reg:        reg,
		requestDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "benchtool",
				Name:      "write_request_duration_seconds",
				Buckets:   []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 120},
			},
			[]string{"code"},
		),
	}

	// Resolve an initial set of distributor addresses
	err := writeBench.ResolveAddrs()
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve enpoints")
	}

	return writeBench, nil
}

func (w *WriteBenchmarkRunner) GetRandomWriteClient() (*WriteClient, error) {
	w.remoteMtx.Lock()
	defer w.remoteMtx.Unlock()

	if len(w.addresses) == 0 {
		return nil, errors.New("no addresses found")
	}
	randomIndex := rand.Intn(len(w.addresses))
	pick := w.addresses[randomIndex]

	var cli *WriteClient
	var exists bool

	if cli, exists = w.clientPool[pick]; !exists {
		u, err := url.Parse("http://" + pick + "/api/v1/push")
		if err != nil {
			return nil, err
		}

		var proxyURL *url.URL
		if w.cfg.ProxyURL != "" {
			proxyURL, err = url.Parse(w.cfg.ProxyURL)
			if err != nil {
				return nil, errors.Wrap(err, "invalid proxy url")
			}
		}

		cli, err = NewWriteClient("bench-"+pick, w.tenantName, &remote.ClientConfig{
			URL:     &config.URL{URL: u},
			Timeout: model.Duration(w.workload.options.Timeout),

			HTTPClientConfig: config.HTTPClientConfig{
				BasicAuth: &config.BasicAuth{
					Username: w.cfg.BasicAuthUsername,
					Password: config.Secret(w.cfg.BasicAuthPasword),
				},
				ProxyURL: config.URL{
					URL: proxyURL,
				},
			},
		}, w.logger, w.requestDuration)
		if err != nil {
			return nil, err
		}
		w.clientPool[pick] = cli
	}

	return cli, nil
}

// Run starts a loop that forwards metrics to the configured remote write endpoint
func (w *WriteBenchmarkRunner) Run(ctx context.Context) error {
	// Start a loop to re-resolve addresses every 5 minutes
	go w.ResolveAddrsLoop(ctx)

	// Run replicas * 10 write client workers.
	// This number will also be used for the number of series buffers to store at once.
	numWorkers := w.workload.Replicas * 10

	batchChan := make(chan BatchReq, 10)
	for i := 0; i < numWorkers; i++ {
		go w.WriteWorker(batchChan)
	}

	return w.workload.GenerateWriteBatch(ctx, w.id, numWorkers+10, batchChan)
}

func (w *WriteBenchmarkRunner) WriteWorker(batchChan chan BatchReq) {
	for batchReq := range batchChan {
		err := w.SendBatch(context.Background(), batchReq.Batch)
		if err != nil {
			level.Warn(w.logger).Log("msg", "unable to send batch", "err", err)
		}

		// put back the series buffer
		batchReq.PutBack <- batchReq.Batch
		batchReq.Wg.Done()
	}
}

func (w *WriteBenchmarkRunner) SendBatch(ctx context.Context, batch []prompb.TimeSeries) error {
	level.Debug(w.logger).Log("msg", "sending timeseries batch", "num_series", strconv.Itoa(len(batch)))
	cli, err := w.GetRandomWriteClient()
	if err != nil {
		return errors.Wrap(err, "unable to get remote-write client")
	}
	req := prompb.WriteRequest{
		Timeseries: batch,
	}

	data, err := req.Marshal()
	if err != nil {
		return errors.Wrap(err, "failed to marshal remote-write request")
	}

	compressed := snappy.Encode(nil, data)

	err = cli.Store(ctx, compressed)

	if err != nil {
		return errors.Wrap(err, "remote-write request failed")
	}

	return nil
}

func (w *WriteBenchmarkRunner) ResolveAddrsLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := w.ResolveAddrs()
			if err != nil {
				level.Warn(w.logger).Log("msg", "failed update remote write servers list", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *WriteBenchmarkRunner) ResolveAddrs() error {
	// Resolve configured addresses with a reasonable timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// If some of the dns resolution fails, log the error.
	if err := w.dnsProvider.Resolve(ctx, []string{w.cfg.Endpoint}); err != nil {
		level.Error(w.logger).Log("msg", "failed to resolve addresses", "err", err)
	}

	// Fail in case no server address is resolved.
	servers := w.dnsProvider.Addresses()
	if len(servers) == 0 {
		return errors.New("no server address resolved")
	}

	w.remoteMtx.Lock()
	w.addresses = servers
	w.remoteMtx.Unlock()

	return nil
}
