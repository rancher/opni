package main

import (
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func ControllerGen() error {
	return sh.RunV(mg.GoCmd(), "run", "sigs.k8s.io/controller-tools/cmd/controller-gen",
		"crd:maxDescLen=0,ignoreUnexportedFields=true",
		"rbac:roleName=manager-role",
		"webhook",
		"object",
		"paths=./apis/...",
		"output:crd:artifacts:config=config/crd/bases",
	)
}
