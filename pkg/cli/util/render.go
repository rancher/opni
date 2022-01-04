package util

import (
	"encoding/hex"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/kralicky/opni-gateway/pkg/management"
)

func RenderBootstrapTokenList(tokens []*management.BootstrapToken) {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "TOKEN", "TTL"})
	for _, t := range tokens {
		token := t.ToToken()
		w.AppendRow(table.Row{token.HexID(), token.EncodeHex(), t.GetTTL()})
	}
	fmt.Println(w.Render())
}

func RenderCertInfoChain(chain []*management.CertInfo) {
	for i, cert := range chain {
		hash := hex.EncodeToString(cert.SPKIHash)
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
				Transformer: func(val interface{}) string {
					if i == len(chain)-1 {
						if str, ok := val.(string); ok && str == hash {
							return text.FgHiGreen.Sprint(val)
						}
					}
					return table.StyleColoredDark.Color.Row.Sprint(val)
				},
			},
		})
		w.AppendRow(table.Row{"SUBJECT", cert.Subject})
		w.AppendRow(table.Row{"ISSUER", cert.Issuer})
		w.AppendRow(table.Row{"CA", cert.IsCA})
		w.AppendRow(table.Row{"NOT BEFORE", cert.NotBefore})
		w.AppendRow(table.Row{"NOT AFTER", cert.NotAfter})
		w.AppendRow(table.Row{"HASH", hash})
		fmt.Println(w.Render())
		if i != len(chain)-1 {
			fmt.Println()
		}
	}
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
