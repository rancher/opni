package targets

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"go/build"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
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
	return ns.run(dagger, takeArgv(arg0)...)
}

func (Dagger) run(pkg daggerPackage, args ...string) error {
	cmds := daggerRun()
	return sh.RunV(cmds[0], append(append(cmds[1:], string(pkg)), args...)...)
}

func (Dagger) do(outputDir string, args ...string) error {
	daggerBinary, err := exec.LookPath("dagger")
	if err != nil {
		return fmt.Errorf("could not find dagger: %w", err)
	}
	return sh.Run(daggerBinary, append([]string{"do", "--output", outputDir, "--project", string(daggerx), "--workdir", string(dagger)}, args...)...)
}

// Invokes 'go run ./dagger --help'
func (ns Dagger) Help() error {
	return sh.RunV(mg.GoCmd(), "run", string(dagger), "--help")
}

// Invokes 'go run ./dagger --setup'
func (Dagger) Setup() error {
	return sh.RunV(mg.GoCmd(), "run", string(dagger), "--setup")
}

// Installs or updates the Dagger CLI to ~/go/bin/dagger
func (Dagger) Install() error {
	modVersion, err := sh.Output(mg.GoCmd(), "list", "-m", "-f", "{{.Version}}", "dagger.io/dagger")
	if err != nil {
		return err
	}

	gopath, goos, goarch := build.Default.GOPATH, build.Default.GOOS, build.Default.GOARCH
	url := fmt.Sprintf("https://github.com/dagger/dagger/releases/download/%[1]s/dagger_%[1]s_%s_%s.tar.gz", modVersion, goos, goarch)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("could not download %s: %s", url, resp.Status)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gzReader.Close()
	tarReader := tar.NewReader(gzReader)
	var header *tar.Header
	for {
		if header, err = tarReader.Next(); err != nil {
			return err
		}
		if header.Name == "dagger" {
			break
		}
	}
	if header == nil {
		return fmt.Errorf("could not find dagger binary in release archive")
	}

	outFilename := path.Join(gopath, "bin", "dagger")
	f, err := os.OpenFile(outFilename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode())
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.CopyN(f, tarReader, header.Size); err != nil {
		return err
	}

	fmt.Printf("Installed dagger %s to %s\n", modVersion, outFilename)
	return nil
}
