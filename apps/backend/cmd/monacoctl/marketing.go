package main

import (
	"context"
	"io"
	"os/signal"
	"syscall"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func toolMarketing(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		return marketing.Main(
			ctx,
			marketing.Deps{Environ: env.environ, Dir: env.wd, Clock: clock.Real{}},
			args,
			stdout,
			stderr,
		)
	}
}
