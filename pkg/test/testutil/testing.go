package testutil

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gexec"
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

type Session interface {
	G() (*gexec.Session, bool)
	Wait() error
}

type sessionWrapper struct {
	g   *gexec.Session
	cmd *exec.Cmd
}

func (s *sessionWrapper) G() (*gexec.Session, bool) {
	if s.g != nil {
		return s.g, true
	}
	return nil, false
}

func (s *sessionWrapper) Wait() error {
	if s == nil {
		return nil
	}
	if s.g != nil {
		ws := s.g.Wait()
		if ws.ExitCode() != 0 {
			return errors.New(string(ws.Err.Contents()))
		}
		return nil
	}
	return s.cmd.Wait()
}

func StartCmd(cmd *exec.Cmd) (Session, error) {
	if IsTesting {
		session, err := gexec.Start(cmd, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
		if err != nil {
			return nil, err
		}
		return &sessionWrapper{
			g:   session,
			cmd: cmd,
		}, nil
	}
	cmd.Stdout = StdoutWriter
	cmd.Stderr = StderrWriter
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &sessionWrapper{
		cmd: cmd,
	}, nil
}
