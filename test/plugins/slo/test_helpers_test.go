package plugins_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/gomega"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func sloCortexGroupsToCheck(groupName string) []string {
	return []string{
		groupName + slo.RecordingRuleSuffix,
		groupName + slo.MetadataRuleSuffix,
		groupName + slo.AlertRuleSuffix,
	}
}

func expectSLOGroupToExist(ctx context.Context, adminClient cortexadmin.CortexAdminClient, tenant string, groupName string) {
	var anyError error
	var wg sync.WaitGroup
	groupsToCheck := sloCortexGroupsToCheck(groupName)
	wg.Add(len(groupsToCheck))

	for _, group := range groupsToCheck {
		groupToCheck := group
		go func() {
			defer wg.Done()
			if err := expectRuleGroupToExist(ctx, adminClient, tenant, groupToCheck); err != nil {
				anyError = err
			}
		}()
	}
	wg.Wait()
	Expect(anyError).Should(BeNil())
}

func expectSLOGroupNotToExist(ctx context.Context, adminClient cortexadmin.CortexAdminClient, tenant string, groupName string) {
	var anyError error
	var wg sync.WaitGroup
	groupsToCheck := sloCortexGroupsToCheck(groupName)
	wg.Add(len(groupsToCheck))

	for _, group := range groupsToCheck {
		groupToCheck := group
		go func() {
			defer wg.Done()
			if err := expectRuleGroupNotToExist(ctx, adminClient, tenant, groupToCheck); err != nil {
				anyError = err
			}
		}()
	}
	wg.Wait()
	Expect(anyError).Should(BeNil())
}

// potentially "long" running function, call asynchronously
func expectRuleGroupToExist(ctx context.Context, adminClient cortexadmin.CortexAdminClient, tenant string, groupName string) error {
	resp, err := adminClient.GetRule(ctx, &cortexadmin.GetRuleRequest{
		ClusterId: tenant,
		Namespace: "test",
		GroupName: groupName,
	})
	if err == nil {
		Expect(resp.Data).To(Not(BeNil()))
		return nil
	}

	return fmt.Errorf("Rule %s should exist, but doesn't", groupName)
}

// potentially "long" running function, call asynchronously
func expectRuleGroupNotToExist(ctx context.Context, adminClient cortexadmin.CortexAdminClient, tenant string, groupName string) error {
	_, err := adminClient.GetRule(ctx, &cortexadmin.GetRuleRequest{
		ClusterId: tenant,
		Namespace: "test",
		GroupName: groupName,
	})
	if err != nil {
		Expect(status.Code(err)).To(Equal(codes.NotFound))
		return nil
	}

	return fmt.Errorf("Rule %s still exists, but shouldn't", groupName)
}
