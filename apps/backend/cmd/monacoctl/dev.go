package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const devUsage = "usage: monacoctl dev token --user <id> [--ttl 24h]\n" +
	"       monacoctl dev privy-token --sub <did:privy:...> | --print-public-key"

func devCmd(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	verbs := map[string]command{"token": devToken, "privy-token": devPrivyToken}
	if len(args) == 0 || verbs[args[0]] == nil {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	return verbs[args[0]](cfg, args[1:], stdout, stderr)
}

func devToken(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dev token", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	user := fs.String("user", "", "user id the token names")
	ttl := fs.Duration("ttl", 24*time.Hour, "how long the token stays valid")
	if err := fs.Parse(args); err != nil || *user == "" || *ttl <= 0 || fs.NArg() != 0 {
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

func devPrivyToken(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dev privy-token", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sub := fs.String("sub", "", "Privy user id the token names")
	publicKey := fs.Bool("print-public-key", false, "print the PEM that PRIVY_VERIFICATION_KEY takes")
	if err := fs.Parse(args); err != nil || (*sub == "") != *publicKey || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	if cfg.Env.Deployed() {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev privy-token: refused with MONACO_ENV=%s\n", cfg.Env)
		return 1
	}
	if *publicKey {
		_, _ = fmt.Fprint(stdout, fakes.PrivyVerificationKey())
		return 0
	}
	_, _ = fmt.Fprintln(stdout, fakes.PrivyAccessToken(cfg.Privy.AppID, *sub, clock.Real{}.Now(), time.Hour))
	return 0
}
