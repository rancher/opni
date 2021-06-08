package opnictl

import (
	"context"
	"io"

	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

func CheckBarFiller(
	waitCtx context.Context,
	success func(context.Context) bool,
) func(mpb.BarFiller) mpb.BarFiller {
	return func(base mpb.BarFiller) mpb.BarFiller {
		done := false
		var doneText string
		return mpb.BarFillerFunc(func(w io.Writer, reqWidth int, st decor.Statistics) {
			if done {
				io.WriteString(w, doneText)
				return
			}
			if st.Completed || waitCtx.Err() != nil {
				done = true
				if success(waitCtx) {
					doneText = chalk.Green.Color("✓")
				} else {
					doneText = chalk.Red.Color("✗")
				}
				io.WriteString(w, doneText)
			} else {
				base.Fill(w, reqWidth, st)
			}
		})
	}
}
