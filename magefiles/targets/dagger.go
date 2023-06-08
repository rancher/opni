package targets

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Dagger mg.Namespace

func (Dagger) Run(args ...string) error {
	dagger, err := exec.LookPath("dagger")
	if err != nil {
		fmt.Fprintf(os.Stderr, "dagger not found in PATH: %v\n", err)
		os.Exit(1)
	}
	return sh.RunV(dagger, append([]string{"run", "go", "run", "./dagger"}, args...)...)
}

func (Dagger) X(args ...string) error {
	dagger, err := exec.LookPath("dagger")
	if err != nil {
		fmt.Fprintf(os.Stderr, "dagger not found in PATH: %v\n", err)
		os.Exit(1)
	}

	return sh.RunV(dagger, append([]string{"run", "go", "run", "./dagger/x"}, args...)...)
}
