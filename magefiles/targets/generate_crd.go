package targets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/samber/lo"
)

type CRD mg.Namespace

func (CRD) All(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.crd")
	defer tr.End()
	mg.SerialCtxDeps(ctx, CRD.CRDGen, CRD.ReplaceCRDText)
}

func (CRD) CRDGen(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.crd.crdgen")
	defer tr.End()
	var commands []*exec.Cmd
	commands = append(commands, exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/kustomize/kustomize/v5",
		"build", "./config/chart-crds", "-o", "./packages/opni/opni/charts/crds/crds.yaml",
	))
	for _, cmd := range commands {
		buf := new(bytes.Buffer)
		cmd.Stderr = buf
		cmd.Stdout = buf
		err := cmd.Run()
		if err != nil {
			if ex, ok := err.(*exec.ExitError); ok {
				if ex.ExitCode() != 1 {
					return errors.New(buf.String())
				}
				bufStr := buf.String()
				lines := strings.Split(bufStr, "\n")
				for _, line := range lines {
					if strings.TrimSpace(line) == "" {
						continue
					}
					fmt.Fprintln(os.Stderr, line)
					return err
				}
			}
		}
	}

	expr := `del(.. | select(has("description")).description) | .. style="flow"`

	e1 := lo.Async(func() error {
		return sh.Run(mg.GoCmd(), "run", "github.com/mikefarah/yq/v4", "-i", expr, "./packages/opni/opni/charts/crds/crds.yaml")
	})

	if err := <-e1; err != nil {
		return err
	}

	// prepend "---" to each file, otherwise kubernetes will think it's json
	for _, f := range []string{"./packages/opni/opni/charts/crds/crds.yaml"} {
		if err := prependDocumentSeparator(f); err != nil {
			return err
		}
	}

	return nil
}

func prependDocumentSeparator(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return err
	}

	buf := make([]byte, i.Size()+4)
	copy(buf[:4], "---\n")

	_, err = f.Read(buf[4:])
	if err != nil {
		return err
	}

	f.Seek(0, 0)
	f.Truncate(0)
	_, err = f.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func (CRD) ReplaceCRDText(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.crd.replacecrdtext")
	defer tr.End()
	files := []string{
		"./packages/opni/opni/charts/crds/crds.yaml",
	}

	for _, file := range files {
		input, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}

		firstReplace := bytes.Replace(input, []byte("replace-me/opni-serving-cert"), []byte(`"replace-me/opni-serving-cert"`), -1)
		output := bytes.Replace(firstReplace, []byte("replace-me"), []byte("{{ .Release.Namespace }}"), -1)

		if err := ioutil.WriteFile(file, output, 0644); err != nil {
			return err
		}
	}
	return nil
}
