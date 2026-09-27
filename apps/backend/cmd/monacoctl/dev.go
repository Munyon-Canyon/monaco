package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const devUsage = "usage: monacoctl dev token --user <id> [--ttl 24h]"

func devCmd(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "token" {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	fs := flag.NewFlagSet("dev token", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	user := fs.String("user", "", "user id the token names")
	ttl := fs.Duration("ttl", 24*time.Hour, "how long the token stays valid")
	if err := fs.Parse(args[1:]); err != nil || *user == "" || *ttl <= 0 || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	clk := clock.Real{}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev token: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, verifier.Mint(*user, clk.Now().Add(*ttl)))
	return 0
}
