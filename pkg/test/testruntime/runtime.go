package testruntime

import (
	"io"
	"os"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
)

var (
	IsTesting       = testing.Testing()
	IsGithubActions = os.Getenv("GITHUB_ACTIONS") == "true"
	IsDrone         = os.Getenv("DRONE") == "true"
	StdoutWriter    = lo.Ternary[io.Writer](IsTesting, ginkgo.GinkgoWriter, os.Stdout)
	StderrWriter    = lo.Ternary[io.Writer](IsTesting, ginkgo.GinkgoWriter, os.Stderr)
)

type IfExpr[T any] struct {
	cond  bool
	value T
}

func IfCI[T any](t T) IfExpr[T] {
	return IfExpr[T]{
		cond:  IsGithubActions || IsDrone,
		value: t,
	}
}

func (e IfExpr[T]) Else(t T) T {
	return lo.Ternary(e.cond, e.value, t)
}

func IfTesting[T any](t T) IfExpr[T] {
	return IfExpr[T]{
		cond:  IsTesting,
		value: t,
	}
}
