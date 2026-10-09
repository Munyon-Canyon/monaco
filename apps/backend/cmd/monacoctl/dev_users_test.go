package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	walletClean = "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"
	usdcMint    = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
)

func localDevConfig() config.Config {
	return config.Config{
		Env: config.EnvLocal, DB: config.DB{URL: "postgres://postgres@localhost:1/monaco"},
		Auth:   config.Auth{DevTokenKey: "test-key"},
		Solana: config.Solana{RPCURL: "http://127.0.0.1:1", USDCMint: usdcMint},
		Timeouts: config.Timeouts{
			Privy: time.Second, RPC: time.Second, RPCBreakerOpen: time.Second,
		},
	}
}

func runDev(cfg config.Config, args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
	code := devCmd(cfg, args, &stdout, &stderr)
	return code, stderr.String()
}

func TestDevUserDelete_refusesBadArgumentsAndNonLocalDatabases(t *testing.T) {
	t.Parallel()
	cfg := localDevConfig()
	for name, args := range map[string][]string{
		"no id": {"user", "delete"}, "wrong verb": {"user", "drop", devUserV7}, "extra": {"user", "delete", devUserV7, "x"},
	} {
		if code, stderr := runDev(cfg, args...); code != 2 || stderr != devUsage+"\n" {
			t.Errorf("%s: exit %d stderr %q", name, code, stderr)
		}
	}
	if code, stderr := runDev(cfg, "user", "delete", devUserV4); code != 2 || stderr != devUserSubjectLine+"\n" {
		t.Errorf("v4 id: exit %d stderr %q", code, stderr)
	}
	for name, mutate := range map[string]func(*config.Config){
		"staging":  func(c *config.Config) { c.Env = config.EnvStaging },
		"test env": func(c *config.Config) { c.Env = config.EnvTest },
		"remote":   func(c *config.Config) { c.DB.URL = "postgres://u@db.supabase.co:5432/monaco" },
	} {
		c := cfg
		mutate(&c)
		if code, stderr := runDev(c, "user", "delete", devUserV7); code != 1 || stderr != devUserRefused+"\n" {
			t.Errorf("%s: exit %d stderr %q", name, code, stderr)
		}
	}
	if code, stderr := runDev(cfg, "user", "delete", devUserV7); code != 1 ||
		!strings.HasPrefix(stderr, "monacoctl dev user delete: ") {
		t.Errorf("database down: exit %d stderr %q", code, stderr)
	}
}

func TestReportDevUserDelete_printsTheOutcome(t *testing.T) {
	t.Parallel()
	id, _ := ids.ParseUserID(devUserV7)
	var stdout, stderr bytes.Buffer
	if code := reportDevUserDelete(id, nil, &stdout, &stderr); code != 0 ||
		stdout.String() != "deleted dev user "+devUserV7+"\n" {
		t.Errorf("success: exit %d stdout %q", code, stdout.String())
	}
	stdout.Reset()
	err := &app.FundedError{Address: walletClean, USDCMicros: 5}
	if code := reportDevUserDelete(id, err, &stdout, &stderr); code != 1 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "monacoctl dev user delete: wallet "+walletClean) {
		t.Errorf("failure: exit %d stderr %q", code, stderr.String())
	}
}

type stubReads struct {
	usdc, sol uint64
	usdcErr   error
	solErr    error
}

func (s stubReads) SOLBalance(context.Context, chain.SolanaAddress) (money.BaseUnits, error) {
	return money.NewBaseUnits(s.sol, 9), s.solErr
}

func (s stubReads) TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error) {
	return money.NewBaseUnits(s.usdc, 6), s.usdcErr
}

func TestBalances_readsUSDCAndSOLAndStopsOnEitherFailure(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeRPCUnavailable, "test.rpc")
	usdc, sol, err := balances{reads: stubReads{usdc: 7, sol: 9}}.Balances(t.Context(), walletClean)
	if usdc != 7 || sol != 9 || err != nil {
		t.Fatalf("Balances = %d, %d, %v", usdc, sol, err)
	}
	for name, reads := range map[string]stubReads{"usdc": {usdcErr: down}, "sol": {solErr: down}} {
		if _, _, err := (balances{reads: reads}).Balances(
			t.Context(),
			walletClean,
		); errs.CodeOf(
			err,
		) != errs.CodeRPCUnavailable {
			t.Errorf("%s failure = %v, want rpc_unavailable", name, err)
		}
	}
	if got := newBalances(localDevConfig()); got.usdc.Decimals != 6 || string(got.usdc.Address) != usdcMint {
		t.Fatalf("newBalances mint = %+v", got.usdc)
	}
}
