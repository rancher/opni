package test

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"emperror.dev/errors"
	"github.com/golang/snappy"
	"github.com/grafana/cortex-tools/pkg/bench"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	"go.uber.org/zap"
)

// Adapted from cortex benchtool

type BenchRunner struct {
	client    remote.WriteClient
	logger    *zap.SugaredLogger
	batchChan chan bench.BatchReq
}

func (e *Environment) NewBenchRunner(agent string, desc bench.WorkloadDesc) (*BenchRunner, error) {
	runningAgent := e.GetAgent(agent)
	if runningAgent.Agent == nil {
		return nil, fmt.Errorf("agent %s not found", agent)
	}
	u, err := url.Parse(fmt.Sprintf("http://%s/api/agent/push", runningAgent.ListenAddress))
	if err != nil {
		return nil, err
	}
	writeClient, err := remote.NewWriteClient(agent, &remote.ClientConfig{
		URL:              &config.URL{URL: u},
		HTTPClientConfig: config.HTTPClientConfig{},
		RetryOnRateLimit: true,
		Timeout:          model.Duration(10 * time.Second),
	})

	workload := bench.NewWriteWorkload(desc, nil)
	batchChan := make(chan bench.BatchReq, 100)
	workload.GenerateWriteBatch(e.ctx, agent, 100, batchChan)

	return &BenchRunner{
		client:    writeClient,
		logger:    e.Logger.Named("bench").With(zap.String("agent", agent)),
		batchChan: batchChan,
	}, nil
}

func (b *BenchRunner) StartWorker(ctx context.Context) {
	go func() {
		b.logger.Info("worker started")
		for {
			select {
			case batchReq := <-b.batchChan:
				if err := b.sendBatch(ctx, batchReq.Batch); err != nil {
					b.logger.With(
						zap.Error(err),
					).Error("failed to send batch")
				}
				batchReq.PutBack <- batchReq.Batch
				batchReq.Wg.Done()
			case <-ctx.Done():
				b.logger.With(
					zap.Error(ctx.Err()),
				).Warn("worker stopped")
				return
			}
		}
	}()
}

func (b *BenchRunner) sendBatch(ctx context.Context, batch []prompb.TimeSeries) error {
	req := prompb.WriteRequest{
		Timeseries: batch,
	}

	data, err := req.Marshal()
	if err != nil {
		return errors.Wrap(err, "failed to marshal remote-write request")
	}

	compressed := snappy.Encode(nil, data)

	return b.client.Store(ctx, compressed)
}
