package cliutil

import (
	"context"
	"io"

	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
)

// CheckBarFiller creates a mpb.BarFiller which can be used to replace a bar
// or spinner with a ✓ or ✗ status indicator based on the return value of the
// provided success function. The success function will be called once when the
// bar is completed or when waitCtx is canceled, whichever happens first.
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
