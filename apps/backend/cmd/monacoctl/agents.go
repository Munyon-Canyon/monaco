package main

import (
	"context"
	"io"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/agents"
)

func toolAgents(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		return agents.Main(context.Background(), env.environ, env.wd, agents.Exec, args, stdout, stderr)
	}
}
