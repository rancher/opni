package testutil

import (
	"errors"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gexec"
	"github.com/rancher/opni/pkg/test/testruntime"
)

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
	if testruntime.IsTesting {
		session, err := gexec.Start(cmd, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
		if err != nil {
			return nil, err
		}
		return &sessionWrapper{
			g:   session,
			cmd: cmd,
		}, nil
	}
	cmd.Stdout = testruntime.StdoutWriter
	cmd.Stderr = testruntime.StderrWriter
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &sessionWrapper{
		cmd: cmd,
	}, nil
}
