package main

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/verify"
)

func verifyTool(environ []string, wd, goBin string) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		target, err := verify.ParseArgs(args)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl verify: %v\n%s\n", err, verify.Usage)
			return 2
		}
		root, err := moduleRoot(wd)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl verify: %v\n", err)
			return 1
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		return verify.Run(ctx, verify.Config{
			Dir: root, Environ: environ, Go: goBin, Docker: "docker",
			Atlas:  filepath.Join(root, "..", "..", ".bin", "atlas"),
			Budget: verify.DefaultBudget(), Stdout: stdout, Stderr: stderr,
		}, target)
	}
}

func toolVerify(env toolEnv) tool { return verifyTool(env.environ, env.wd, "go") }
