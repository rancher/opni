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

func daggerBinary() (string, error) {
	dagger, err := exec.LookPath("dagger")
	if err != nil {
		return "", fmt.Errorf("dagger not found: %w", err)
	}
	return dagger, nil
}

type daggerPackage string

const (
	dagger  daggerPackage = "./dagger"
	daggerx daggerPackage = "./dagger/x"
)

// Invokes 'dagger run go run ./dagger' with all arguments
func (ns Dagger) Run(arg0 string) error {
	return ns.run("./dagger", ns.takeArgv(arg0)...)
}

// Invokes 'dagger run go run ./dagger/x' with all arguments
func (ns Dagger) X(arg0 string) error {
	return ns.do("./dagger/x", ns.takeArgv(arg0)...)
}

func (Dagger) run(pkg daggerPackage, args ...string) error {
	dagger, err := daggerBinary()
	if err != nil {
		return err
	}
	return sh.RunV(dagger, append([]string{"run", "go", "run", string(pkg)}, args...)...)
}

func (Dagger) do(pkg daggerPackage, args ...string) error {
	dagger, err := daggerBinary()
	if err != nil {
		return err
	}
	return sh.Run(dagger, append([]string{"do", "--project", "./dagger", "--config", string(pkg)}, args...)...)
}

func (Dagger) takeArgv(arg0 string) (rest []string) {
	idx := slices.Index(os.Args, arg0)
	rest, os.Args = os.Args[idx:], os.Args[:idx]
	return
}
