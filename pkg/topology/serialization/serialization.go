package serialization

import (
	"crypto/md5"
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
)

var (
	//go:embed templates/*.tmpl
	templateFiles embed.FS
	Templates     *template.Template
)

func init() {
	Templates = template.New("output").Funcs(template.FuncMap{
		"json": func(i interface{}) string {
			b, err := json.Marshal(i)
			if err != nil {
				return err.Error()
			}
			return string(b)
		},
		"yaml": func(i interface{}) string {
			b, err := yaml.Marshal(i)
			if err != nil {
				return err.Error()
			}
			return strings.Trim(string(b), "\n")
		},
		"underscore": func(s string) string {
			re := regexp.MustCompile(`[^A-Za-z0-9]+`)
			return re.ReplaceAllString(strings.ToLower(s), "_")
		},
		"color": func(s string) string {
			hash := md5.Sum([]byte(s))
			return fmt.Sprintf("#%x", hash[:3])
		},
	})
	template.Must(Templates.ParseFS(templateFiles, "templates/*.tmpl"))
}

type GraphSeralization interface {
	Serialize([]byte) ([]byte, error)
	Deserialize([]byte) ([]byte, error)
}
