package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	identitysqlc "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const devPrivyMaxMicros = 5_000_000

var (
	errPrivyActionEnded   = errors.New("privy action did not succeed")
	errPrivyActionPending = errors.New("privy action still pending")
)

func devPrivyEVMWallet(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dev privy-evm-wallet", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	key := fs.String("key", "", "idempotency key; the same key returns the same wallet")
	if err := fs.Parse(args); err != nil || *key == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	if cfg.Env == config.EnvProduction {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev privy-evm-wallet: refused with MONACO_ENV=%s\n", cfg.Env)
		return 1
	}
	client, err := privy.New(cfg, clock.Real{})
	if err == nil {
		var wallet multichain.EVMWallet
		if wallet, err = client.CreateAppEVMWallet(context.Background(), *key); err == nil {
			_, _ = fmt.Fprintf(stdout, "wallet %s\naddress %s\n", wallet.ID, wallet.Address)
			return 0
		}
	}
	_, _ = fmt.Fprintf(stderr, "monacoctl dev privy-evm-wallet: %v\n", err)
	return 1
}

func devPrivyTransfer(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dev privy-transfer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	user := fs.String("user", "", "Monaco user whose member wallet sends")
	wallet := fs.String("wallet", "", "Privy wallet id that sends, instead of --user")
	to := fs.String("to", "", "destination address")
	network := fs.String("network", "base", "destination network")
	usdc := fs.String("usdc", "", "USDC to send, at most 5")
	key := fs.String("key", "", "idempotency key; a rerun with the same key finds the same action")
	timeout := fs.Duration("timeout", 15*time.Minute, "how long to poll the action")
	poll := fs.Duration("poll", 5*time.Second, "how often to read the action")
	micros, okAmount := uint64(0), false
	if err := fs.Parse(args); err == nil {
		micros, okAmount = parseUSDC(*usdc)
	}
	dest, netErr := chain.ParseNetwork(*network)
	if !okAmount || micros == 0 || micros > devPrivyMaxMicros || (*user == "") == (*wallet == "") || *to == "" ||
		*key == "" || netErr != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	if cfg.Env == config.EnvProduction {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev privy-transfer: refused with MONACO_ENV=%s\n", cfg.Env)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := runPrivyTransfer(ctx, cfg, privyTransferArgs{
		user: *user, wallet: *wallet, key: *key, poll: *poll,
		spec: multichain.TransferSpec{
			From: chain.NetworkSolana, To: dest, Destination: *to, AmountMicros: money.MicrosFromUint64(micros),
		},
	}, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev privy-transfer: %v\n", err)
		return 1
	}
	return 0
}

type privyTransferArgs struct {
	user, wallet, key string
	poll              time.Duration
	spec              multichain.TransferSpec
}

func runPrivyTransfer(ctx context.Context, cfg config.Config, in privyTransferArgs, stdout io.Writer) error {
	walletID := in.wallet
	if in.user != "" {
		id, err := memberWalletID(ctx, cfg, in.user)
		if err != nil {
			return err
		}
		walletID = id
	}
	client, err := privy.New(cfg, clock.Real{})
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]any{"wallet_id": walletID, "spec": in.spec, "idempotency_key": in.key})
	_, _ = fmt.Fprintf(stdout, "request %s\n", request)
	id, err := client.Transfer(ctx, walletID, in.spec, in.key)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "action %s\n", id)
	return pollPrivyAction(ctx, client, walletID, id, in.poll, stdout)
}

func pollPrivyAction(
	ctx context.Context, client *privy.Client, walletID string, id multichain.ActionID, poll time.Duration,
	stdout io.Writer,
) error {
	var last []byte
	for {
		status, err := client.Action(ctx, walletID, id)
		if err != nil {
			return err
		}
		if line, _ := json.Marshal(status); string(line) != string(last) {
			_, _ = fmt.Fprintf(stdout, "status %s\n", line)
			last = line
		}
		switch status.State {
		case multichain.ActionSucceeded:
			return nil
		case multichain.ActionPending:
		case multichain.ActionRejected, multichain.ActionFailed:
			return fmt.Errorf("action %s ended %s: %w", id, status.State, errPrivyActionEnded)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("action %s: %w: %w", id, errPrivyActionPending, ctx.Err())
		case <-time.After(poll):
		}
	}
}

func memberWalletID(ctx context.Context, cfg config.Config, raw string) (string, error) {
	user, err := uuid.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("--user: %w", err)
	}
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return "", err
	}
	defer pool.Close()
	row, err := identitysqlc.New(pool).MemberWalletByUserID(ctx, user)
	if err != nil {
		return "", fmt.Errorf("member wallet of %s: %w", user, err)
	}
	return row.PrivyWalletID, nil
}
