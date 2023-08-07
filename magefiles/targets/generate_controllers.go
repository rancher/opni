package targets

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
)

// Generates code and CRDs for kubebuilder apis
func (Generate) Controllers(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.generate.controllers")
	defer tr.End()

	var typesSources []string
	err := filepath.WalkDir("apis", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), "_types.go") {
			typesSources = append(typesSources, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	ok, err := target.Dir("config/crd/bases", typesSources...)
	if err != nil {
		return err
	}
	if ok {
		return sh.RunWithV(map[string]string{
			"CGO_ENABLED": "1",
		}, mg.GoCmd(), "run", "sigs.k8s.io/controller-tools/cmd/controller-gen@latest",
			"crd:maxDescLen=0,ignoreUnexportedFields=true,allowDangerousTypes=true",
			"rbac:roleName=manager-role",
			"object",
			"paths=./apis/...",
			"output:crd:artifacts:config=config/crd/bases",
		)
	}
	return nil
}
