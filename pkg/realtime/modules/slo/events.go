package slo

import (
	"context"
	"time"

	"github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type sloEventType int

const (
	sloAdded sloEventType = iota
	sloRemoved
	sloUpdated
)

type sloEvent struct {
	slo *slo.ServiceLevelObjective
	typ sloEventType
}

func (m *module) watchEvents(ctx context.Context) {
	interval := time.Second * 1
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	current := map[string]*slo.ServiceLevelObjective{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c, ca := context.WithTimeout(ctx, interval/2)
			list, err := m.sloClient.ListSLOs(c, &emptypb.Empty{})
			ca()
			if err != nil {
				continue
			}
			latest := lo.KeyBy(list.Items, (*slo.ServiceLevelObjective).GetId)
			added := lo.PickBy(latest, func(k string, _ *slo.ServiceLevelObjective) bool {
				_, ok := current[k]
				return !ok
			})
			removed := lo.PickBy(current, func(k string, _ *slo.ServiceLevelObjective) bool {
				_, ok := latest[k]
				return !ok
			})
			updated := lo.PickBy(latest, func(k string, v *slo.ServiceLevelObjective) bool {
				slo, ok := current[k]
				if !ok {
					return false
				}
				return !proto.Equal(slo, v)
			})
			current = latest
			for _, slo := range added {
				m.events <- &sloEvent{
					slo: slo,
					typ: sloAdded,
				}
			}
			for _, slo := range removed {
				m.events <- &sloEvent{
					slo: slo,
					typ: sloRemoved,
				}
			}
			for _, slo := range updated {
				m.events <- &sloEvent{
					slo: slo,
					typ: sloUpdated,
				}
			}
		}
	}
}
