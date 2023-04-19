package discovery

import (
	"sort"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusDiscovery struct {
	logger     *zap.SugaredLogger
	retrievers []ScrapeConfigRetriever
}

type jobs = map[string]yaml.MapSlice

type ScrapeConfigRetriever interface {
	Yield() (jobs, error)
	Name() string
}

func NewPrometheusDiscovery(
	logger *zap.SugaredLogger,
	client client.Client,
) PrometheusDiscovery {
	return PrometheusDiscovery{
		logger: logger,
		retrievers: []ScrapeConfigRetriever{
			NewServiceMonitorScrapeConfigRetriever(logger, client),
			NewPodMonitorScrapeConfigRetriever(logger, client),
		},
	}
}

func (p *PrometheusDiscovery) YieldScrapeConfigs() (scrapeCfgs []yaml.MapSlice, retErr error) {
	cfgChan := make(chan map[string]yaml.MapSlice)

	go func() {
		var wg sync.WaitGroup
		defer close(cfgChan)
		for _, retriever := range p.retrievers {
			retriever := retriever
			lg := p.logger.With("collector", retriever.Name())
			wg.Add(1)
			go func() {
				defer wg.Done()
				jobs, err := retriever.Yield()
				if err != nil {
					lg.Warnf("failed to retrieve scrape configs : %s", err)
					return
				}
				lg.Debugf("found %d scrape configs", len(jobs))
				cfgChan <- jobs
			}()
		}
		wg.Wait()

	}()
	jobMap := map[string]yaml.MapSlice{}
	for {
		cfg, ok := <-cfgChan
		if len(cfg) > 0 {
			jobMap = lo.Assign(jobMap, cfg)
		}
		if !ok {
			break
		}
	}
	jobs := lo.Keys(jobMap)
	sort.Strings(jobs)

	for _, job := range jobs {
		scrapeCfgs = append(scrapeCfgs, jobMap[job])
	}
	p.logger.Debugf("reduced found scrape configs to %d", len(scrapeCfgs))
	return scrapeCfgs, nil
}

func parseStrToDuration(s string) model.Duration {
	dur, err := time.ParseDuration(s)
	if err != nil {
		return model.Duration(time.Minute)
	}
	return model.Duration(dur)
}
