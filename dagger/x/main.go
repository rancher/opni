package main

import (
	"encoding/json"

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
	opts.MountOnly = false
	ctr := cmds.TestBin(client, client.Container(), opts)

	return ctr.Directory("/src/testbin"), nil
}
