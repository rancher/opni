package capabilities

import (
	"bytes"
	"strings"
	"text/template"
)

type InstallerTemplateSpec struct {
	Token   string
	Pin     string
	Address string
}

func RenderInstallerCommand(tmpl string, spec InstallerTemplateSpec) (string, error) {
	t, err := template.New("installer").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(tmpl)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	err = t.Execute(buf, spec)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
