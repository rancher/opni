package targets

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"golang.org/x/exp/slices"
)

type Dagger mg.Namespace

func daggerRun() []string {
	dagger, err := exec.LookPath("dagger")
	if err != nil {
		return []string{"go", "run"}
	}
	return []string{dagger, "run", "go", "run"}
}

type daggerPackage string

const (
	dagger  daggerPackage = "./dagger"
	daggerx daggerPackage = "./dagger/x"
)

// Invokes 'go run ./dagger' with all arguments
func (ns Dagger) Run(arg0 string) error {
	return ns.run(dagger, ns.takeArgv(arg0)...)
}

func (Dagger) run(pkg daggerPackage, args ...string) error {
	cmds := daggerRun()
	return sh.RunV(cmds[0], append(append(cmds[1:], string(pkg)), args...)...)
}

func (Dagger) do(pkg daggerPackage, args ...string) error {
	dagger, err := exec.LookPath("dagger")
	if err != nil {
		return fmt.Errorf("could not find dagger: %w", err)
	}
	return sh.Run(dagger, append([]string{"do", "--project", dagger, "--config", string(pkg)}, args...)...)
}

func (Dagger) takeArgv(arg0 string) (rest []string) {
	idx := slices.Index(os.Args, arg0)
	rest, os.Args = os.Args[idx:], os.Args[:idx]
	return
}

// Invokes 'go run ./dagger --help'
func (ns Dagger) Help() error {
	return sh.RunV(mg.GoCmd(), "run", string(dagger), "--help")
}

// Invokes 'go run ./dagger --setup'
func (Dagger) Setup() error {
	return sh.RunV(mg.GoCmd(), "run", string(dagger), "--setup")
}
