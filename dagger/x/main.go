package main

import (
	"encoding/json"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/x/cmds"
)

func main() {
	dagger.ServeCommands(
		Testbin,
	)
}

func Testbin(ctx dagger.Context, config string) (*dagger.Directory, error) {
	client := ctx.Client()
	opts := cmds.TestBinOptions{}
	if err := json.Unmarshal([]byte(config), &opts); err != nil {
		return nil, err
	}
	opts.MountOnly = true
	ctr := cmds.TestBin(client, client.Container(), opts)

	return client.Directory().
		WithNewDirectory("testbin/bin").
		With(func(dir *dagger.Directory) *dagger.Directory {
			for _, b := range opts.Binaries {
				dir = dir.WithFile(filepath.Join("testbin/bin", b.Name), ctr.File(filepath.Join("/src/testbin/bin/", b.Name)))
			}
			return dir.WithFile("testbin/lock.json", ctr.File("/src/testbin/lock.json"))
		}), nil
}
