package otel

import (
	"embed"
	"strings"
	"text/template"

	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	//go:embed templates/*
	otelTemplateFs embed.FS

	OTELTemplates *template.Template

	otelTemplateFuncs = template.FuncMap{
		"protoDurToString": ProtoDurToString,
		"indent":           indent,
		"nindent":          nindent,
	}
)

func init() {
	OTELTemplates = util.Must(template.New("otel").Funcs(otelTemplateFuncs).ParseFS(otelTemplateFs, "templates/*.tmpl"))

}

func ProtoDurToString(dur *durationpb.Duration) string {
	return model.Duration(dur.AsDuration().Nanoseconds()).String()
}

func indent(spaces int, v string) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.Replace(v, "\n", "\n"+pad, -1)
}

func nindent(spaces int, v string) string {
	return "\n" + indent(spaces, v)
}
