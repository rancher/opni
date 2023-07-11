package mock_rules

import (
	"context"

	"github.com/rancher/opni/pkg/rules"
	mock_notifier "github.com/rancher/opni/pkg/test/mock/notifier"
	"github.com/rancher/opni/pkg/util/notifier"
	"go.uber.org/mock/gomock"
)

func NewTestFinder(ctrl *gomock.Controller, groups func() []rules.RuleGroup) notifier.Finder[rules.RuleGroup] {
	mockRuleFinder := mock_notifier.NewMockFinder[rules.RuleGroup](ctrl)
	mockRuleFinder.EXPECT().
		Find(gomock.Any()).
		DoAndReturn(func(ctx context.Context) ([]rules.RuleGroup, error) {
			return groups(), nil
		}).
		AnyTimes()
	return mockRuleFinder
}
