package main

import (
	"os"

	bindata "github.com/go-bindata/go-bindata"
	"github.com/sirupsen/logrus"
)

var (
	basePackage = "github.com/rancher/opnictl/types"
)

func main() {
	os.Unsetenv("GOPATH")

	bc := &bindata.Config{
		Input: []bindata.InputConfig{
			{
				Path:      "manifests",
				Recursive: true,
			},
		},
		Package:    "deploy",
		NoMetadata: true,
		Prefix:     "manifests/",
		Output:     "pkg/deploy/zz_generated_bindata.go",
		Tags:       "!no_stage",
	}
	if err := bindata.Translate(bc); err != nil {
		logrus.Fatal(err)
	}
}
