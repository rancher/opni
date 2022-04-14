package storage_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/storage"
)

func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage Suite")
}

func cluster(id string, labels ...string) *core.Cluster {
	cluster := &core.Cluster{
		Id: id,
		Metadata: &core.ClusterMetadata{
			Labels: map[string]string{},
		},
	}
	for i := 0; i < len(labels); i += 2 {
		cluster.Metadata.Labels[labels[i]] = labels[i+1]
	}
	return cluster
}

func selector(idsOrSelectorOrOptions ...interface{}) storage.ClusterSelector {
	var ids []string
	var selector *core.LabelSelector
	var options core.MatchOptions
	for _, arg := range idsOrSelectorOrOptions {
		switch value := arg.(type) {
		case string:
			ids = append(ids, value)
		case []string:
			ids = append(ids, value...)
		case *core.LabelSelector:
			selector = value
		case core.MatchOptions:
			options |= value
		}
	}
	return storage.ClusterSelector{
		ClusterIDs:    ids,
		LabelSelector: selector,
		MatchOptions:  options,
	}
}

func matchLabels(labels ...string) *core.LabelSelector {
	ls := &core.LabelSelector{
		MatchLabels: map[string]string{},
	}
	for i := 0; i < len(labels); i += 2 {
		ls.MatchLabels[labels[i]] = labels[i+1]
	}
	return ls
}

func matchExprs(exprs ...string) *core.LabelSelector {
	ls := &core.LabelSelector{}
	for _, expr := range exprs {
		parts := strings.Split(expr, " ")
		switch len(parts) {
		case 3:
			ls.MatchExpressions = append(ls.MatchExpressions, &core.LabelSelectorRequirement{
				Key:      parts[0],
				Operator: parts[1],
				Values:   strings.Split(parts[2], ","),
			})
		case 2:
			ls.MatchExpressions = append(ls.MatchExpressions, &core.LabelSelectorRequirement{
				Key:      parts[0],
				Operator: parts[1],
			})
		}
	}
	return ls
}

type rbacObjects struct {
	roles        []func() *core.Role
	roleBindings []func() *core.RoleBinding
}

func rbacs(objects ...interface{}) rbacObjects {
	objs := rbacObjects{}
	for _, o := range objects {
		switch v := o.(type) {
		case func() *core.Role:
			objs.roles = append(objs.roles, v)
		case func() *core.RoleBinding:
			objs.roleBindings = append(objs.roleBindings, v)
		}
	}
	return objs
}

func role(id string, clusterIdOrSelector ...interface{}) func() *core.Role {
	return func() *core.Role {
		r := &core.Role{
			Id: id,
		}
		for _, i := range clusterIdOrSelector {
			switch v := i.(type) {
			case string:
				r.ClusterIDs = append(r.ClusterIDs, v)
			case []string:
				r.ClusterIDs = append(r.ClusterIDs, v...)
			case *core.LabelSelector:
				r.MatchLabels = v
			}
		}
		return r
	}
}

var rbacStore storage.RBACStore

func rb(id string, roleName string, subjects ...string) func() *core.RoleBinding {
	return func() *core.RoleBinding {
		rb := &core.RoleBinding{
			Id:       id,
			RoleId:   roleName,
			Subjects: subjects,
		}
		storage.ApplyRoleBindingTaints(context.Background(), rbacStore, rb)
		return rb
	}
}
