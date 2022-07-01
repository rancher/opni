package notifier_test

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/model/rulefmt"
	mock_notifier "github.com/rancher/opni/pkg/test/mock/notifier"
	"github.com/rancher/opni/pkg/util/notifier"
)

//FIXME:
type MockClone struct {
	a int
}

func (c MockClone) Clone() MockClone {
	return MockClone{c.a}
}

var _ = Describe("Periodic Update Notifier", Label("unit", "temporal"), func() {
	It("should periodically fetch rules", func() {
		ctrl := gomock.NewController(GinkgoT())
		// finder := mock_rules.NewMockRuleFinder(ctrl)
		finder := mock_notifier.NewMockFinder[MockClone](ctrl)
		tc := make(chan time.Time, 100)
		finder.EXPECT().
			Find(gomock.Any()).
			DoAndReturn(func(ctx context.Context) ([]rulefmt.RuleGroup, error) {
				tc <- time.Now()
				return []rulefmt.RuleGroup{}, nil
			}).
			MinTimes(10)
		interval := 10 * time.Millisecond
		ctx, ca := context.WithTimeout(context.Background(), interval*12)
		defer ca()
		//FIXME:
		notifier := notifier.NewPeriodicUpdateNotifier(ctx, (notifier.Finder[MockClone])(finder), interval)
		go func() {
			ch := notifier.NotifyC(context.Background())
			for {
				select {
				case <-ctx.Done():
					return
				case <-ch:
				}
			}
		}()
		<-ctx.Done()
		ctrl.Finish()

		// ensure all timestamps are ~interval apart
		timestamps := make([]time.Time, 0, len(tc))
	READ:
		for {
			select {
			case t, ok := <-tc:
				if !ok {
					break READ
				}
				timestamps = append(timestamps, t)
			default:
				break READ
			}
		}
		for i := 1; i < len(tc); i++ {
			Expect(timestamps[i].Sub(timestamps[i-1])).To(BeNumerically("~", interval, interval/10))
		}
	})
})
