package cliutil

import (
	"reflect"
	"runtime"

	"github.com/spf13/cobra"
)

func BasePreRunE(cmd *cobra.Command, args []string) error {
	return base(func(c *cobra.Command) func(*cobra.Command, []string) error {
		return c.PersistentPreRunE
	}, cmd, args)
}

func BasePostRunE(p *cobra.Command, args []string) error {
	return base(func(c *cobra.Command) func(*cobra.Command, []string) error {
		return c.PersistentPostRunE
	}, p, args)
}

func base(field func(*cobra.Command) func(*cobra.Command, []string) error, cmd *cobra.Command, args []string) error {
	// https://github.com/spf13/cobra/issues/1198

	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		panic("could not get caller")
	}
	callingFunc := runtime.FuncForPC(pc)

	for p := cmd.Parent(); p != nil; p = p.Parent() {
		argWoFlags := p.Flags().Args()
		if p.DisableFlagParsing {
			argWoFlags = args
		}

		if fp := field(p); fp != nil {
			// check if p.PersistentPreRunE is the calling function
			// since PersistentPreRunE is called by walking up the command tree
			// from the leaf command, we could end up calling the same function
			// we started at, since `cmd` is the leaf command
			if runtime.FuncForPC(reflect.ValueOf(fp).Pointer()) == callingFunc {
				continue
			}

			if p.Context() == nil {
				p.SetContext(cmd.Context())
				defer func() {
					if p.Context() != cmd.Context() {
						cmd.SetContext(p.Context())
					}
				}()
			}
			if err := fp(p, argWoFlags); err != nil {
				return err
			}
			break
		}
	}

	return nil

}
