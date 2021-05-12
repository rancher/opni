package main

import (
	"os"
	"regexp"

	bindata "github.com/go-bindata/go-bindata"
	"github.com/sirupsen/logrus"
)

var (
	basePackage = "github.com/rancher/opnictl/types"
)

func main() {
	os.Unsetenv("GOPATH")
	// ignore anything in src dir but yaml
	yamlOnly := regexp.MustCompile(`.*(\.py|\.sh|\.md|Dockerfile|\.txt|json)`)
	bc := &bindata.Config{
		Input: []bindata.InputConfig{
			{
				Path:      "src/k8s",
				Recursive: true,
			},
		},
		Package:    "deploy",
		NoMetadata: true,
		Prefix:     "src/k8s",
		Output:     "pkg/deploy/zz_generated_bindata.go",
		Tags:       "!no_stage",
		Ignore:     []*regexp.Regexp{yamlOnly},
	}
	if err := bindata.Translate(bc); err != nil {
		logrus.Fatal(err)
	}
}
