package amtool

import (
	"os"

	"github.com/prometheus/alertmanager/cli"
)

func Execute(args []string) {
	os.Args = append([]string{"amtool"}, args...)
	cli.Execute()
}
