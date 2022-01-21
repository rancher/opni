package util

import (
	"bytes"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/kralicky/opni-monitoring/pkg/management"
)

func RenderBootstrapTokenList(tokens []*management.BootstrapToken) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "TOKEN", "TTL"})
	for _, t := range tokens {
		token := t.ToToken()
		w.AppendRow(table.Row{token.HexID(), token.EncodeHex(), t.GetTTL()})
	}
	return w.Render()
}

func RenderCertInfoChain(chain []*management.CertInfo) string {
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

func RenderTenantList(tenants []*management.Tenant) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID"})
	for _, t := range tenants {
		w.AppendRow(table.Row{t.ID})
	}
	return w.Render()
}

func RenderRole(role *management.Role) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"NAME", "TENANTS"})
	for i, tenant := range role.TenantIDs {
		name := role.Name
		if i > 0 {
			name = ""
		}
		w.AppendRow(table.Row{name, tenant})
	}
	return w.Render()
}

func RenderRoleList(roles []*management.Role) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"NAME", "# TENANTS"})
	for _, r := range roles {
		w.AppendRow(table.Row{r.Name, len(r.TenantIDs)})
	}
	return w.Render()
}

func RenderRoleBinding(binding *management.RoleBinding) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"NAME", "ROLE NAME", "USER ID"})
	w.AppendRow(table.Row{binding.Name, binding.RoleName, binding.UserID})
	return w.Render()
}

func RenderRoleBindingList(bindings []*management.RoleBinding) string {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"NAME", "ROLE NAME", "USER ID"})
	for _, b := range bindings {
		w.AppendRow(table.Row{b.Name, b.RoleName, b.UserID})
	}
	return w.Render()
}

type AccessMatrix struct {
	// List of users (in the order they will appear in the table)
	Users []string
	// Map of tenant IDs to a set of users that have access to the tenant
	TenantsToUsers map[string]map[string]struct{}
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
	for tenant, users := range am.TenantsToUsers {
		row = table.Row{tenant}
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
	return w.Render()
}
