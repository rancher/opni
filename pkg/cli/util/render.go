package util

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/tokens"
	"github.com/ttacon/chalk"
)

func RenderBootstrapToken(token *core.BootstrapToken) string {
	return RenderBootstrapTokenList(&core.BootstrapTokenList{
		Items: []*core.BootstrapToken{token},
	})
}

func RenderBootstrapTokenList(list *core.BootstrapTokenList) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "TOKEN", "TTL", "USAGES", "LABELS"})
	for _, t := range list.Items {
		token, err := tokens.FromBootstrapToken(t)
		if err != nil {
			return err.Error()
		}

		w.AppendRow(table.Row{
			token.HexID(),
			token.EncodeHex(),
			(time.Duration(t.GetMetadata().GetTtl()) * time.Second).String(),
			t.GetMetadata().GetUsageCount(),
			strings.Join(JoinKeyValuePairs(t.GetMetadata().GetLabels()), "\n"),
		})
	}
	return w.Render()
}

func RenderCertInfoChain(chain []*core.CertInfo) string {
	buf := new(bytes.Buffer)
	for i, cert := range chain {
		fp := []byte(cert.Fingerprint)
		w := table.NewWriter()
		w.SetIndexColumn(1)
		w.SetStyle(table.StyleColoredDark)
		w.SetColumnConfigs([]table.ColumnConfig{
			{
				Number: 1,
				Align:  text.AlignRight,
			},
			{
				Number: 2,
			},
		})
		w.AppendRow(table.Row{"SUBJECT", cert.Subject})
		w.AppendRow(table.Row{"ISSUER", cert.Issuer})
		w.AppendRow(table.Row{"IS CA", cert.IsCA})
		w.AppendRow(table.Row{"NOT BEFORE", cert.NotBefore})
		w.AppendRow(table.Row{"NOT AFTER", cert.NotAfter})
		w.AppendRow(table.Row{"FINGERPRINT", string(fp)})
		buf.WriteString(w.Render())
		if i < len(chain)-1 {
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

func RenderClusterList(list *core.ClusterList) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "LABELS"})
	for _, t := range list.Items {
		labels := []string{}
		for k, v := range t.GetMetadata().GetLabels() {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		w.AppendRow(table.Row{t.GetId(), strings.Join(labels, ",")})
	}
	return w.Render()
}

func RenderRole(role *core.Role) string {
	return RenderRoleList(&core.RoleList{
		Items: []*core.Role{role},
	})
}

func RenderRoleList(list *core.RoleList) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "SELECTOR", "CLUSTER IDS"})
	for _, role := range list.Items {
		clusterIds := strings.Join(role.ClusterIDs, "\n")
		if len(clusterIds) == 0 {
			clusterIds = "(none)"
		}
		expressionStr := role.MatchLabels.ExpressionString()
		if expressionStr == "" {
			expressionStr = "(none)"
		}
		w.AppendRow(table.Row{role.Id, expressionStr, clusterIds})
	}
	return w.Render()
}

func RenderRoleBinding(binding *core.RoleBinding) string {
	return RenderRoleBindingList(&core.RoleBindingList{
		Items: []*core.RoleBinding{binding},
	})
}

func RenderRoleBindingList(list *core.RoleBindingList) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	header := table.Row{"ID", "ROLE ID", "SUBJECTS"}
	anyRolesHaveTaints := false
	for _, rb := range list.Items {
		if len(rb.Taints) > 0 {
			anyRolesHaveTaints = true
		}
	}
	if anyRolesHaveTaints {
		header = append(header, "TAINTS")
	}
	w.AppendHeader(header)
	for _, b := range list.Items {
		row := table.Row{b.Id, b.RoleId, strings.Join(b.Subjects, "\n")}
		if anyRolesHaveTaints {
			row = append(row, chalk.Red.Color(strings.Join(b.Taints, "\n")))
		}
		w.AppendRow(row)
	}
	return w.Render()
}

type AccessMatrix struct {
	// List of users (in the order they will appear in the table)
	Users []string
	// Set of known clusters (rules referencing nonexistent clusters are marked)
	KnownClusters map[string]struct{}
	// Map of tenant IDs to a set of users that have access to the tenant
	ClustersToUsers map[string]map[string]struct{}
}

func RenderAccessMatrix(am AccessMatrix) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.Style().Format = table.FormatOptions{
		Footer: text.FormatUpper,
		Header: text.FormatDefault,
		Row:    text.FormatDefault,
	}
	w.SortBy([]table.SortBy{
		{
			Number: 1,
		},
	})
	// w.SetIndexColumn(1)
	cc := []table.ColumnConfig{
		{
			Number:      1,
			AlignHeader: text.AlignCenter,
		},
	}
	if len(am.Users) == 0 {
		return ""
	}
	row := table.Row{"TENANT ID"}
	for i, user := range am.Users {
		row = append(row, user)
		cc = append(cc, table.ColumnConfig{
			Number:      i + 2,
			AlignHeader: text.AlignCenter,
			Align:       text.AlignCenter,
		})
	}
	w.SetColumnConfigs(cc)
	w.AppendHeader(row)
	needsFootnote := false
	for cluster, users := range am.ClustersToUsers {
		clusterText := cluster
		if _, ok := am.KnownClusters[cluster]; !ok {
			needsFootnote = true
			clusterText = fmt.Sprintf("%s*", cluster)
		}
		row = table.Row{clusterText}
		for _, user := range am.Users {
			if _, ok := users[user]; ok {
				// print unicode checkmark
				row = append(row, "\u2705")
			} else {
				row = append(row, "\u274C")
			}
		}
		w.AppendRow(row)
	}
	if needsFootnote {
		w.AppendFooter(table.Row{"Clusters marked with * are not known to the server."})
	}
	return w.Render()
}
