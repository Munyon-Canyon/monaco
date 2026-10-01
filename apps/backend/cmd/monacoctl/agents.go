package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/agents"
)

func toolAgents(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return agents.Main(ctx, env.environ, env.wd, agents.Exec, args, stdout, stderr)
	}
}
