package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	testflows "github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func devTokenNewUser(cfg config.Config, ttl time.Duration, stdout, stderr io.Writer) int {
	clk := clock.Real{}
	verifier, err := auth.NewDevVerifier(cfg, clk)
	if err != nil {
		return devTokenFail(stderr, err)
	}
	user, err := testflows.NewDevUser(context.Background(), cfg)
	if err != nil {
		return devTokenFail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, verifier.Mint(user.UserID.String(), clk.Now().Add(ttl)))
	_, _ = fmt.Fprintf(stderr, "dev user %s @%s wallet %s\n", user.UserID, user.Handle, user.WalletAddress)
	return 0
}

func devTokenFail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "monacoctl dev token: %v\n", err)
	return 1
}
