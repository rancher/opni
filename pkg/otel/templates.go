package otel

import (
	"embed"
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
	}
)

func init() {
	OTELTemplates = util.Must(template.New("otel").Funcs(otelTemplateFuncs).ParseFS(otelTemplateFs, "templates/*.tmpl"))

}

func ProtoDurToString(dur *durationpb.Duration) string {
	return model.Duration(dur.AsDuration().Nanoseconds()).String()
}
