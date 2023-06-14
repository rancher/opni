package alarms

import (
	"context"

	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (a *AlarmServerComponent) SyncRules(ctx context.Context, rules *rules.RuleManifest) (*emptypb.Empty, error) {
	id := cluster.StreamAuthorizedID(ctx)
	a.logger.Debugf("received %d rules from agent %s", len(rules.Rules), id)
	return &emptypb.Empty{}, nil
}
