package util

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ServerListener interface {
	Listen() error
	Shutdown() error
}

// Starts the server by calling Listen() and waits for SIGHUP or a server
// error. If SIGHUP is received, this function returns nil. If a server error
// occurs, this function returns the error.
func ListenReload(l ServerListener) error {
	errC := make(chan error)
	sigC := make(chan os.Signal, 1)
	go func() { errC <- l.Listen() }()
	signal.Notify(sigC, syscall.SIGHUP)
	defer func() {
		signal.Stop(sigC)
		close(sigC)
		close(errC)
	}()

	// handle SIGHUP
	select {
	case err := <-errC:
		return err
	case <-sigC:
		log.Println("SIGHUP received, restarting")
		if err := l.Shutdown(); err != nil {
			log.Fatalf("failed to shut down listener: %v", err)
		}
		select {
		case <-time.After(5 * time.Second):
			log.Fatalf("failed to shut down listener: timeout")
		case <-errC:
		}
		return nil
	}
}
