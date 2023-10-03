package cliutil

import (
	"fmt"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/samber/lo"
	"github.com/ttacon/chalk"
)

func RenderBootstrapToken(token *corev1.BootstrapToken) string {
	return RenderBootstrapTokenList(&corev1.BootstrapTokenList{
		Items: []*corev1.BootstrapToken{token},
	})
}

func RenderBootstrapTokenList(list *corev1.BootstrapTokenList) string {
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

func RenderCertInfoChain(chain []*corev1.CertInfo) string {
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
	for i, cert := range chain {
		fp := []byte(cert.Fingerprint)
		w.AppendRow(table.Row{"SUBJECT", cert.Subject})
		w.AppendRow(table.Row{"ISSUER", cert.Issuer})
		w.AppendRow(table.Row{"IS CA", cert.IsCA})
		w.AppendRow(table.Row{"NOT BEFORE", cert.NotBefore})
		w.AppendRow(table.Row{"NOT AFTER", cert.NotAfter})
		w.AppendRow(table.Row{"FINGERPRINT", string(fp)})
		if i < len(chain)-1 {
			w.AppendSeparator()
		}
	}
	return w.Render()
}

func RenderClusterList(list *corev1.ClusterList, status []*corev1.HealthStatus) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	renderSessionAttributes := false
	renderInstanceInfo := false
	for _, s := range status {
		if len(s.GetStatus().GetSessionAttributes()) > 0 {
			renderSessionAttributes = true
			break
		}
	}
	for _, c := range list.Items {
		if c.GetMetadata().GetLastKnownConnectionDetails().GetInstanceInfo() != nil {
			renderInstanceInfo = true
			break
		}
	}
	hdr := table.Row{"ID", "LABELS", "CAPABILITIES", "STATUS"}
	if renderSessionAttributes {
		hdr = append(hdr, "ATTRIBUTES")
	}
	if renderInstanceInfo {
		hdr = append(hdr, "INSTANCE")
	}
	w.AppendHeader(hdr)
	for i, t := range list.Items {
		labels := []string{}
		for k, v := range t.GetMetadata().GetLabels() {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		capabilities := []string{}
		for _, c := range t.GetCapabilities() {
			if c.DeletionTimestamp == nil {
				capabilities = append(capabilities, c.Name)
			} else {
				capabilities = append(capabilities, fmt.Sprintf("%s (deleting)", c.Name))
			}
		}
		row := table.Row{t.GetId(), strings.Join(labels, ","), strings.Join(capabilities, ","), status[i].Summary()}
		if renderSessionAttributes {
			row = append(row, strings.Join(status[i].Status.GetSessionAttributes(), ","))
		}
		if renderInstanceInfo {
			instanceInfo := t.GetMetadata().GetLastKnownConnectionDetails().GetInstanceInfo()
			if instanceInfo != nil && instanceInfo.GetAcquired() {
				row = append(row, instanceInfo.GetRelayAddress())
			} else {
				row = append(row, "")
			}
		}
		w.AppendRow(row)
	}
	return w.Render()
}

func RenderRole(role *corev1.Role) string {
	return RenderRoleList(&corev1.RoleList{
		Items: []*corev1.Role{role},
	})
}

func RenderRoleList(list *corev1.RoleList) string {
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

func RenderRoleBinding(binding *corev1.RoleBinding) string {
	return RenderRoleBindingList(&corev1.RoleBindingList{
		Items: []*corev1.RoleBinding{binding},
	})
}

func RenderRoleBindingList(list *corev1.RoleBindingList) string {
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

func RenderMetricSamples(samples []*model.Sample) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	if len(samples) == 0 {
		return ""
	}
	header := table.Row{"namespace", "cluster", "blocks"}

	w.AppendHeader(header)
	for _, s := range samples {
		user := s.Metric["user"]
		if user == "rules" {
			continue
		}
		w.AppendRow(table.Row{s.Metric["namespace"], user, s.Value})
	}
	return w.Render()
}

func RenderClusterDetails(cluster *corev1.Cluster) string {
	w := table.NewWriter()
	w.SetIndexColumn(1)
	w.SetStyle(table.StyleColoredDark)
	labels := cluster.GetLabels()
	name, ok := labels[corev1.NameLabel]
	if ok {
		w.SetTitle(fmt.Sprintf("%s [%s]", name, cluster.Id))
	} else {
		w.SetTitle(cluster.Id)
	}
	if !cluster.GetCreationTimestamp().IsZero() {
		ts := cluster.GetCreationTimestamp().In(time.Local)
		w.AppendRow(table.Row{"Created",
			fmt.Sprintf("%s (%s ago)", ts.Format(time.RFC822), time.Since(ts).Round(time.Second)),
		})
	}
	if len(cluster.GetLabels()) > 0 {
		labelPairs := lo.ToPairs(cluster.GetLabels())
		w.AppendRow(table.Row{"Labels", labelPairs[0].Key, labelPairs[0].Value})
		for _, v := range labelPairs[1:] {
			w.AppendRow(table.Row{"", v.Key, v.Value})
		}
	}
	if len(cluster.GetCapabilities()) > 0 {
		w.AppendSeparator()
		w.AppendRow(table.Row{"Capabilities", "Name", "Status"})
		for _, c := range cluster.GetCapabilities() {
			status := "OK"
			if c.DeletionTimestamp != nil {
				status = "Deleting"
			}
			w.AppendRow(table.Row{"", c.Name, status})
		}
	}
	w.AppendSeparator()
	lkcd := cluster.GetMetadata().GetLastKnownConnectionDetails()
	if lkcd != nil {
		w.AppendRow(table.Row{"Last Known Connection", "Connection Time", lkcd.GetTime().AsTime().Local().Format(time.RFC3339)})
		w.AppendRow(table.Row{"", "Peer Address", lkcd.GetAddress()})
		if lkcd.AgentBuildInfo != nil {
			w.AppendSeparator()
			w.AppendRow(table.Row{"Build Info", "Go Version", lkcd.AgentBuildInfo.GetGoVersion()})
			w.AppendRow(table.Row{"", "Path", lkcd.AgentBuildInfo.GetMain().GetPath()})
			w.AppendRow(table.Row{"", "Version", lkcd.AgentBuildInfo.GetMain().GetVersion()})
			w.AppendRow(table.Row{"", "Sum", lkcd.AgentBuildInfo.GetMain().GetSum()})
			if len(lkcd.AgentBuildInfo.Settings) > 0 {
				w.AppendSeparator()
				w.AppendRow(table.Row{"Build Settings", lkcd.AgentBuildInfo.Settings[0].Key, lkcd.AgentBuildInfo.Settings[0].Value})
				for _, s := range lkcd.AgentBuildInfo.Settings[1:] {
					w.AppendRow(table.Row{"", s.Key, s.Value})
				}
			}
		}
		if len(lkcd.GetPluginVersions()) > 0 {
			w.AppendSeparator()
			pluginPairs := lo.ToPairs(lkcd.GetPluginVersions())

			w.AppendRow(table.Row{"Manifest", pluginPairs[0].Key, pluginPairs[0].Value})
			for _, v := range pluginPairs[1:] {
				w.AppendRow(table.Row{"", v.Key, v.Value})
			}
		}
	}
	return w.Render()
}
