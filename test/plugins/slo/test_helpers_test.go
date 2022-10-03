package plugins_test

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"time"
)

func sloCortexGroupsToCheck(groupName string) []string {
	return []string{
		groupName + slo.RecordingRuleSuffix,
		groupName + slo.MetadataRuleSuffix,
		groupName + slo.AlertRuleSuffix,
	}
}

func expectSLOGroupToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) {
	var anyError error
	var wg sync.WaitGroup
	groupsToCheck := sloCortexGroupsToCheck(groupName)
	wg.Add(len(groupsToCheck))

	for _, group := range groupsToCheck {
		groupToCheck := group
		go func() {
			defer wg.Done()
			if err := expectRuleGroupToExist(adminClient, ctx, tenant, groupToCheck); err != nil {
				anyError = err
			}
		}()
	}
	wg.Wait()
	Expect(anyError).Should(BeNil())
}

func expectSLOGroupNotToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) {
	var anyError error
	var wg sync.WaitGroup
	groupsToCheck := sloCortexGroupsToCheck(groupName)
	wg.Add(len(groupsToCheck))

	for _, group := range groupsToCheck {
		groupToCheck := group
		go func() {
			defer wg.Done()
			if err := expectRuleGroupNotToExist(adminClient, ctx, tenant, groupToCheck); err != nil {
				anyError = err
			}
		}()
	}
	wg.Wait()
	Expect(anyError).Should(BeNil())
}

// potentially "long" running function, call asynchronously
func expectRuleGroupToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) error {
	for i := 0; i < 10; i++ {
		resp, err := adminClient.GetRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: tenant,
			GroupName: groupName,
		})
		if err == nil {
			Expect(resp.Data).To(Not(BeNil()))
			return nil
		}
		time.Sleep(1)
	}
	return fmt.Errorf("Rule %s should exist, but doesn't", groupName)
}

// potentially "long" running function, call asynchronously
func expectRuleGroupNotToExist(adminClient cortexadmin.CortexAdminClient, ctx context.Context, tenant string, groupName string) error {
	for i := 0; i < 10; i++ {
		_, err := adminClient.GetRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: tenant,
			GroupName: groupName,
		})
		if err != nil {
			Expect(status.Code(err)).To(Equal(codes.NotFound))
			return nil
		}

		time.Sleep(1)
	}
	return fmt.Errorf("Rule %s still exists, but shouldn't", groupName)
}
