package test

import (
	"github.com/kralicky/spellbook/build"
	"github.com/kralicky/spellbook/testbin"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func Env() {
	mg.Deps(testbin.Testbin, build.Build)
	sh.RunV("bin/testenv")
}
