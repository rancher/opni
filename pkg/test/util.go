package test

import (
	"context"
	"fmt"
	"github.com/goombaio/namegenerator"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

func RandomName(seed int64) string {
	nameGenerator := namegenerator.NewNameGenerator(seed)
	return nameGenerator.Generate()
}

func ExpectRuleGroupToExist(
	client cortexadmin.CortexAdminClient,
	ctx context.Context,
	tenant string,
	groupName string,
	pollInterval time.Duration,
	maxTimeout time.Duration,
) {
	Eventually(func() error {
		_, err := client.GetRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: tenant,
			GroupName: groupName,
		})
		if err != nil {
			return err
		}
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("rule group %s not found", groupName)
		}
		return nil
	}, maxTimeout, pollInterval).Should(Succeed())
}
