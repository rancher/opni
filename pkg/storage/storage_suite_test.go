package storage_test

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	_ "github.com/rancher/opni/pkg/test/setup"
)

func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage Suite")
}

func cluster(id string, labels ...string) *corev1.Cluster {
	cluster := &corev1.Cluster{
		Id: id,
		Metadata: &corev1.ClusterMetadata{
			Labels: map[string]string{},
		},
	}
	for i := 0; i < len(labels); i += 2 {
		cluster.Metadata.Labels[labels[i]] = labels[i+1]
	}
	return cluster
}

func selector(idsOrSelectorOrOptions ...interface{}) *corev1.ClusterSelector {
	var ids []string
	var selector *corev1.LabelSelector
	var options corev1.MatchOptions
	for _, arg := range idsOrSelectorOrOptions {
		switch value := arg.(type) {
		case string:
			ids = append(ids, value)
		case []string:
			ids = append(ids, value...)
		case *corev1.LabelSelector:
			selector = value
		case corev1.MatchOptions:
			options |= value
		}
	}
	return &corev1.ClusterSelector{
		ClusterIDs:    ids,
		LabelSelector: selector,
		MatchOptions:  options,
	}
}

func matchLabels(labels ...string) *corev1.LabelSelector {
	ls := &corev1.LabelSelector{
		MatchLabels: map[string]string{},
	}
	for i := 0; i < len(labels); i += 2 {
		ls.MatchLabels[labels[i]] = labels[i+1]
	}
	return ls
}

func matchExprs(exprs ...string) *corev1.LabelSelector {
	ls := &corev1.LabelSelector{}
	for _, expr := range exprs {
		parts := strings.Split(expr, " ")
		switch len(parts) {
		case 3:
			ls.MatchExpressions = append(ls.MatchExpressions, &corev1.LabelSelectorRequirement{
				Key:      parts[0],
				Operator: parts[1],
				Values:   strings.Split(parts[2], ","),
			})
		case 2:
			ls.MatchExpressions = append(ls.MatchExpressions, &corev1.LabelSelectorRequirement{
				Key:      parts[0],
				Operator: parts[1],
			})
		}
	}
	return ls
}

type rbacObjects struct {
	roles        []func() *corev1.Role
	roleBindings []func() *corev1.RoleBinding
}

func rbacs(objects ...interface{}) rbacObjects {
	objs := rbacObjects{}
	for _, o := range objects {
		switch v := o.(type) {
		case func() *corev1.Role:
			objs.roles = append(objs.roles, v)
		case func() *corev1.RoleBinding:
			objs.roleBindings = append(objs.roleBindings, v)
		}
	}
	return objs
}

func role(id string, clusterIdOrSelector ...interface{}) func() *corev1.Role {
	return func() *corev1.Role {
		r := &corev1.Role{
			Id: id,
		}
		for _, i := range clusterIdOrSelector {
			switch v := i.(type) {
			case string:
				appendClusterIDsToRole(r, v)
			case []string:
				appendClusterIDsToRole(r, v...)
			case *corev1.LabelSelector:
				r.Permissions = append(r.Permissions, &corev1.PermissionItem{
					Type:        string(corev1.PermissionTypeCluster),
					Verbs:       []*corev1.PermissionVerb{corev1.VerbGet()},
					MatchLabels: v,
				})
			}
		}
		return r
	}
}

func appendClusterIDsToRole(role *corev1.Role, ids ...string) {
	for _, permission := range role.GetPermissions() {
		if permission.Type == string(corev1.PermissionTypeCluster) && corev1.VerbGet().InList(permission.GetVerbs()) {
			permission.Ids = append(permission.Ids, ids...)
			return
		}
	}
	role.Permissions = append(role.Permissions, &corev1.PermissionItem{
		Type:  string(corev1.PermissionTypeCluster),
		Verbs: []*corev1.PermissionVerb{corev1.VerbGet()},
		Ids:   ids,
	})
}
