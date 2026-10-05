package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const devUsage = "usage: monacoctl dev token (--user <id> | --user new | --new-user) [--ttl 24h]\n" +
	"       monacoctl dev privy-token --sub <did:privy:...> | --print-public-key\n" +
	"       monacoctl dev seed-scenario <name> [--actor A=<user-uuid> ...] [--json]"

const devUserSubjectLine = "monacoctl dev token: --user must be a version 7 UUID, the only user id the API accepts"

func devCmd(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	verbs := map[string]command{"token": devToken, "privy-token": devPrivyToken, "seed-scenario": devSeedScenario}
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
	newUser := fs.Bool("new-user", false, "mint for a fresh version 7 user id")
	ttl := fs.Duration("ttl", 24*time.Hour, "how long the token stays valid")
	if err := fs.Parse(args); err != nil || *ttl <= 0 || fs.NArg() != 0 || (*user != "") == *newUser {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	if *user == "new" {
		return devTokenNewUser(cfg, *ttl, stdout, stderr)
	}
	subject := *user
	if *newUser {
		subject = ids.NewUserID(ids.Real{}).String()
	} else if _, err := ids.ParseUserID(subject); err != nil {
		_, _ = fmt.Fprintln(stderr, devUserSubjectLine)
		return 2
	}
	clk := clock.Real{}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev token: %v\n", err)
		return 1
	}
	token := verifier.Mint(subject, clk.Now().Add(*ttl))
	if *newUser {
		_, _ = fmt.Fprintf(stdout, "%s\n%s\n", token, subject)
		return 0
	}
	_, _ = fmt.Fprintln(stdout, token)
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
