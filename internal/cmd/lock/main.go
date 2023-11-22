package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rancher/opni/internal/lock"
)

func main() {
	ctx, ca := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	rootCmd := lock.BuildRootCmd()
	go func() {
		<-c
		ca()
		<-c
		os.Exit(1)
	}()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
