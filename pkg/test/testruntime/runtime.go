package testruntime

import (
	"io"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
)

var (
	IsTesting       = strings.HasSuffix(os.Args[0], ".test")
	IsGithubActions = os.Getenv("GITHUB_ACTIONS") == "true"
	IsDrone         = os.Getenv("DRONE") == "true"
	StdoutWriter    = lo.Ternary[io.Writer](IsTesting, ginkgo.GinkgoWriter, os.Stdout)
	StderrWriter    = lo.Ternary[io.Writer](IsTesting, ginkgo.GinkgoWriter, os.Stderr)
)

type ifExpr[T any] struct {
	cond  bool
	value T
}

func IfCI[T any](t T) ifExpr[T] {
	return ifExpr[T]{
		cond:  IsGithubActions || IsDrone,
		value: t,
	}
}

func (e ifExpr[T]) Else(t T) T {
	return lo.Ternary(e.cond, e.value, t)
}

func IfTesting[T any](t T) ifExpr[T] {
	return ifExpr[T]{
		cond:  IsTesting,
		value: t,
	}
}
