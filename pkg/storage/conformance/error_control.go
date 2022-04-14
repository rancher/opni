package conformance

import (
	"os"
	"syscall"
)

type ErrorController interface {
	EnableErrors()
	DisableErrors()
}

type errorControllerImpl struct {
	enable  func()
	disable func()
}

func (e *errorControllerImpl) EnableErrors() {
	e.enable()
}

func (e *errorControllerImpl) DisableErrors() {
	e.disable()
}

func NewErrorController(enable, disable func()) ErrorController {
	return &errorControllerImpl{
		enable:  enable,
		disable: disable,
	}
}

func NewProcessErrorController(proc *os.Process) ErrorController {
	return NewErrorController(
		func() {
			proc.Signal(syscall.SIGSTOP)
		},
		func() {
			proc.Signal(syscall.SIGCONT)
		},
	)
}
