package slo

import (
	"context"
	"io"

	"github.com/kralicky/gpkg/sync"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
)

type task interface {
	Run(ctx context.Context, state io.ReadWriter)
	InitMetrics() []prometheus.Collector
}

type runningTask struct {
	task   task
	cancel context.CancelFunc
}

type newTaskFunc func(*slo.ServiceLevelObjective) task

func (m *module) manageTasks(ctx context.Context, newTask newTaskFunc) {
	tasks := sync.Map[string, *runningTask]{}

	start := func(slo *slo.ServiceLevelObjective) {
		tctx, tca := context.WithCancel(ctx)
		running := &runningTask{
			task:   newTask(slo),
			cancel: tca,
		}
		tasks.Store(slo.GetId(), running)
		metrics := running.task.InitMetrics()
		go func() {
			for _, metric := range metrics {
				m.mc.Reg.Register(metric)
			}
			running.task.Run(tctx, &stateReadWriter{
				ctx:       tctx,
				id:        slo.GetId(),
				sloClient: m.sloClient,
			})
			for _, metric := range metrics {
				m.mc.Reg.Unregister(metric)
			}
		}()
	}
	stop := func(slo *slo.ServiceLevelObjective) {
		if value, ok := tasks.LoadAndDelete(slo.GetId()); ok {
			value.cancel()
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.events:
			clone := proto.Clone(event.slo).(*slo.ServiceLevelObjective)
			switch event.typ {
			case sloAdded:
				start(clone)
			case sloRemoved:
				stop(clone)
			case sloUpdated:
				stop(clone)
				start(clone)
			}
		}
	}
}
