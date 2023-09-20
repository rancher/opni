//go:build !minimal

package render

import (
	"fmt"
	"strings"

	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"google.golang.org/protobuf/encoding/prototext"

	"slices"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/samber/lo"
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

func RenderTargetList(list *remoteread.TargetList) string {
	writer := table.NewWriter()
	writer.SetStyle(table.StyleColoredDark)
	writer.AppendHeader(table.Row{"CLUSTER", "NAME", "ENDPOINT", "LAST READ", "STATE", "MESSAGE"})

	for _, target := range list.Targets {
		var state string
		switch target.Status.State {
		case remoteread.TargetState_Running:
			state = "running"
		case remoteread.TargetState_Failed:
			state = "failed"
		case remoteread.TargetState_Canceled:
			state = "canceled"
		case remoteread.TargetState_Completed:
			state = "complete"
		case remoteread.TargetState_NotRunning:
			state = "not running"
		default:
			state = "unknown"
		}

		// todo: we should be able to accept whatever time format given here as the --start parameter to opni import start
		var lastRead string
		if progress := target.Status.Progress; progress != nil {
			lastRead = progress.LastReadTimestamp.AsTime().String()
		}

		row := table.Row{target.Meta.ClusterId, target.Meta.Name, target.Spec.Endpoint, lastRead, state, target.Status.Message}

		writer.AppendRow(row)
	}

	return writer.Render()
}

func RenderDiscoveryEntries(entries []*remoteread.DiscoveryEntry) string {
	writer := table.NewWriter()
	writer.SetStyle(table.StyleColoredDark)
	writer.AppendHeader(table.Row{"CLUSTER", "NAME", "EXTERNAL", "INTERNAL"})

	for _, entry := range entries {
		row := table.Row{entry.ClusterId, entry.Name, entry.ExternalEndpoint, entry.InternalEndpoint}
		writer.AppendRow(row)
	}

	return writer.Render()
}

type MetricsNodeConfigInfo struct {
	Id            string
	HasCapability bool
	Spec          *node.MetricsCapabilitySpec
	IsDefault     bool
}

func RenderMetricsNodeConfigs(nodes []MetricsNodeConfigInfo, defaultConfig *node.MetricsCapabilitySpec) string {
	writer := table.NewWriter()
	writer.SetStyle(table.StyleColoredDark)
	writer.AppendHeader(table.Row{"ID", "HAS CAPABILITY", "CONFIG"})
	writer.Style().Format = table.FormatOptions{
		Footer: text.FormatDefault,
		Header: text.FormatDefault,
		Row:    text.FormatDefault,
	}
	writer.SetColumnConfigs([]table.ColumnConfig{
		{
			Number:      1,
			Align:       text.AlignLeft,
			AlignFooter: text.AlignLeft,
			AlignHeader: text.AlignLeft,
		},
		{
			Number:      3,
			Align:       text.AlignLeft,
			AlignFooter: text.AlignLeft,
			AlignHeader: text.AlignLeft,
		},
	})
	marshal := prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
		EmitASCII: true,
	}

	for _, node := range nodes {
		var spec string
		if node.IsDefault {
			spec = "(default)"
		} else if node.Spec != nil {
			spec = marshal.Format(node.Spec)
		} else {
			spec = "(nil)"
		}
		row := table.Row{node.Id, node.HasCapability, spec}
		writer.AppendRow(row)
	}

	if defaultConfig != nil {
		writer.AppendFooter(table.Row{"(default)", "", marshal.Format(defaultConfig)})
	}

	return writer.Render()
}

func RenderDefaultNodeConfig(defaultConfig *node.MetricsCapabilitySpec) string {
	marshal := prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
		EmitASCII: true,
	}
	return marshal.Format(defaultConfig)
}
