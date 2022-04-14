package test

import (
	"github.com/kralicky/spellbook/build"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func Env() {
	mg.Deps(build.Build)
	sh.RunV("bin/testenv")
}
