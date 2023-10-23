package v1

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/samber/lo"
)

func (cl *CapabilityList) Names() []string {
	names := []string{}
	for _, c := range cl.Items {
		names = append(names, c.Details.Name)
	}
	return names
}

func (m *ListClustersRequest) CacheKey() string {
	if m.MatchLabels == nil && m.MatchOptions == 0 {
		return "all"
	}
	if len(m.MatchLabels.MatchExpressions) == 0 &&
		len(m.MatchLabels.MatchLabels) == 0 &&
		m.MatchOptions == 0 {
		return "all"
	}
	key := fmt.Sprintf("%d", m.MatchOptions)
	for _, l := range m.MatchLabels.GetMatchExpressions() {
		key += l.Key + l.Operator + strings.Join(l.Values, "")
	}
	matchLabelKeys := lo.Keys(m.MatchLabels.GetMatchLabels())
	sort.Strings(matchLabelKeys)
	for _, mKey := range matchLabelKeys {
		key += mKey + m.MatchLabels.MatchLabels[mKey]
	}
	return key
}

func RenderCapabilityList(list *CapabilityList) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"NAME", "SOURCE", "DRIVERS", "CLUSTERS"})
	for _, c := range list.Items {
		drivers := c.GetDetails().GetAvailableDrivers()
		enabledDriver := c.GetDetails().GetEnabledDriver()
		if len(drivers) > 0 {
			slices.SortFunc(drivers, func(a, b string) int {
				if a == enabledDriver {
					return -1
				}
				return strings.Compare(a, b)
			})
			if drivers[0] == enabledDriver {
				drivers[0] = fmt.Sprintf("%s (enabled)", drivers[0])
			}
		}
		w.AppendRow(table.Row{
			c.GetDetails().GetName(),
			c.GetDetails().GetSource(),
			strings.Join(drivers, ", "), // ex: "driver1 (enabled), driver2, driver3"
			c.GetNodeCount(),
		})
	}
	return w.Render()
}
