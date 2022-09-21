//go:build !noplugins

package cliutil

import (
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

func RenderClusterListWithStats(list *corev1.ClusterList, status []*corev1.HealthStatus, stats *cortexadmin.UserIDStatsList) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	if stats == nil {
		w.AppendHeader(table.Row{"ID", "LABELS", "CAPABILITIES", "STATUS"})
	} else {
		w.AppendHeader(table.Row{"ID", "LABELS", "CAPABILITIES", "STATUS", "NUM SERIES", "SAMPLE RATE", "RULE RATE"})
	}
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
		if stats != nil {
			for _, s := range stats.Items {
				if string(s.UserID) == t.GetId() {
					row = append(row,
						fmt.Sprint(s.NumSeries),
						fmt.Sprintf("%.1f/s", s.APIIngestionRate),
						fmt.Sprintf("%.1f/s", s.RuleIngestionRate),
					)
					break
				}
			}
		}
		w.AppendRow(row)
	}
	return w.Render()
}

func RenderCortexClusterStatus(status *cortexadmin.CortexStatus) string {
	tables := []string{
		renderCortexServiceStatus(status),
	}
	if status.GetDistributor().GetIngesterRing().GetEnabled() {
		tables = append(tables, renderShardStatus("Distributor Ingester Ring", status.GetDistributor().GetIngesterRing().GetShards()))
	}
	if status.GetIngester().GetRing().GetEnabled() {
		tables = append(tables, renderShardStatus("Ingester Ring", status.GetIngester().GetRing().GetShards()))
	}
	if status.GetRuler().GetRing().GetEnabled() {
		tables = append(tables, renderShardStatus("Ruler Ring", status.GetRuler().GetRing().GetShards()))
	}
	if status.GetCompactor().GetRing().GetEnabled() {
		tables = append(tables, renderShardStatus("Compactor Ring", status.GetCompactor().GetRing().GetShards()))
	}
	if status.GetStoreGateway().GetRing().GetEnabled() {
		tables = append(tables, renderShardStatus("Store Gateway Ring", status.GetStoreGateway().GetRing().GetShards()))
	}

	if status.GetIngester().GetMemberlist().GetEnabled() {
		tables = append(tables, renderMemberlistStatus("Ingester Memberlist", status.GetIngester().GetMemberlist().GetMembers()))
	}
	if status.GetRuler().GetMemberlist().GetEnabled() {
		tables = append(tables, renderMemberlistStatus("Ruler Memberlist", status.GetRuler().GetMemberlist().GetMembers()))
	}
	if status.GetCompactor().GetMemberlist().GetEnabled() {
		tables = append(tables, renderMemberlistStatus("Compactor Memberlist", status.GetCompactor().GetMemberlist().GetMembers()))
	}
	if status.GetStoreGateway().GetMemberlist().GetEnabled() {
		tables = append(tables, renderMemberlistStatus("Store Gateway Memberlist", status.GetStoreGateway().GetMemberlist().GetMembers()))
	}
	if status.GetQuerier().GetMemberlist().GetEnabled() {
		tables = append(tables, renderMemberlistStatus("Querier Memberlist", status.GetQuerier().GetMemberlist().GetMembers()))
	}

	return strings.Join(tables, "\n\n")
}

func renderCortexServiceStatus(status *cortexadmin.CortexStatus) string {
	w := table.NewWriter()
	w.SetTitle("Cortex Services")
	w.SetStyle(table.StyleColoredDark)
	w.SetIndexColumn(1)
	w.Style().Format = table.FormatOptions{
		Footer: text.FormatDefault,
		Header: text.FormatDefault,
		Row:    text.FormatDefault,
	}
	w.SetColumnConfigs([]table.ColumnConfig{
		{
			Number: 1,
			Align:  text.AlignRight,
		},
	})
	w.SortBy([]table.SortBy{
		{
			Number: 1,
			Mode:   table.Asc,
		},
	})

	services := map[string]map[string]string{}

	services["Distributor"] = servicesByName(status.Distributor)
	services["Ingester"] = servicesByName(status.Ingester)
	services["Ruler"] = servicesByName(status.Ruler)
	services["Purger"] = servicesByName(status.Purger)
	services["Compactor"] = servicesByName(status.Compactor)
	services["Store Gateway"] = servicesByName(status.StoreGateway)
	services["Querier"] = servicesByName(status.Querier)

	moduleNames := []string{}
	for _, v := range services {
		moduleNames = append(moduleNames, lo.Keys(v)...)
	}
	moduleNames = lo.Uniq(moduleNames)
	slices.Sort(moduleNames)

	header := table.Row{""}
	for _, module := range moduleNames {
		header = append(header, module)
	}
	w.AppendHeader(header)

	for svcName, status := range services {
		row := table.Row{svcName}
		for _, mod := range moduleNames {
			row = append(row, status[mod])
		}
		w.AppendRow(row)
	}

	return w.Render()
}

func renderShardStatus(title string, status *cortexadmin.ShardStatusList) string {
	w := table.NewWriter()
	w.SetTitle(title)
	w.SetStyle(table.StyleColoredDark)
	w.SetIndexColumn(1)

	header := table.Row{"ID", "STATE", "ADDRESS", "TIMESTAMP"}
	w.AppendHeader(header)

	for _, instance := range status.GetShards() {
		row := table.Row{
			instance.GetId(),
			instance.GetState(),
			instance.GetAddress(),
			instance.GetTimestamp(),
		}
		w.AppendRow(row)
	}

	return w.Render()
}

func renderMemberlistStatus(title string, status *cortexadmin.MemberStatusList) string {
	w := table.NewWriter()
	w.SetTitle(title)
	w.SetStyle(table.StyleColoredDark)
	w.SetIndexColumn(1)

	header := table.Row{"NAME", "ADDRESS", "PORT", "STATE"}
	w.AppendHeader(header)

	for _, instance := range status.GetItems() {
		row := table.Row{
			instance.GetName(),
			instance.GetAddress(),
			instance.GetPort(),
			instance.GetState(),
		}
		w.AppendRow(row)
	}

	return w.Render()
}

func servicesByName[T interface {
	GetServices() *cortexadmin.ServiceStatusList
}](t T) map[string]string {
	services := map[string]string{}
	for _, s := range t.GetServices().GetServices() {
		services[s.GetName()] = s.GetStatus()
	}
	return services
}
