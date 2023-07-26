package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"
)

type Writer interface {
	Print(i ...any)
	Println(i ...any)
	Printf(format string, i ...any)
	PrintErr(i ...any)
	PrintErrln(i ...any)
	PrintErrf(format string, i ...any)
}

// An optional interface that can be implemented by an RPC response type to
// control how it is rendered to the user.
type TextRenderer interface {
	// RenderText renders the message to the given writer in a human-readable
	// format.
	RenderText(out Writer)
}

func RenderOutput(cmd *cobra.Command, response proto.Message) {
	outputFormat, _ := cmd.Flags().GetString("output")
	outWriter := cmd.OutOrStdout()
	if outputFormat == "auto" {
		if file, ok := outWriter.(*os.File); ok {
			if term.IsTerminal(int(file.Fd())) {
				outputFormat = "text"
			} else {
				outputFormat = "json"
			}
		}
	}

	switch outputFormat {
	case "json":
		fmt.Fprintln(outWriter, protojson.MarshalOptions{}.Format(response))
	case "json,multiline":
		fmt.Fprintln(outWriter, protojson.MarshalOptions{
			Multiline: true,
		}.Format(response))
	case "yaml":
		out, err := yaml.Marshal(response)
		if err != nil {
			cmd.PrintErrln(err)
			return
		}
		fmt.Fprintln(outWriter, string(out))
	case "text":
		if renderer, ok := response.(TextRenderer); ok {
			renderer.RenderText(cmd)
			return
		}
		fmt.Fprintln(outWriter, prototext.MarshalOptions{
			Multiline: true,
		}.Format(response))
	default:
		cmd.PrintErrln("Unknown output format:", outputFormat)
	}
}

func AddOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("output", "o", "auto", "Output format (json[,multiline]|yaml|text|auto)")
}
