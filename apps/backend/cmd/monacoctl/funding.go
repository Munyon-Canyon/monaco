package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	fundingapp "github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const fundingUsage = `usage: monacoctl funding bounce <command> <external-deposit-id> [args]
  set-return-address <id> <address>  send the bounce to <address>; only while detected or bounce_failed
  retry <id>                         sign and send a new bounce for a bounce_failed deposit
  hold <id>                          keep a detected or bounce_failed deposit as unclaimed and end its pause

usage: monacoctl funding bounce list [status ...]
  lists external deposits by status, oldest first: id, cabal, status, amount, mint, return address, attempts,
  detected at. The default is detected, bouncing and bounce_failed.

usage: monacoctl funding withdrawal list-support
  lists every EVM withdrawal in submitted whose Privy action failed past a broadcast step: id, user, amount, age,
  source signature, action id. It reads the database and Privy, so it survives a restart.

usage: monacoctl funding withdrawal resolve <withdrawal-id> --to confirmed|failed --reason <text>
    [--refund-signature <signature>] [--override]
  settles an EVM withdrawal stuck in submitted after its Privy action failed past a broadcast step. confirmed means
  support confirmed the funds reached the destination. failed releases the member's hold, so it is refused when the
  chain shows the funds left (Solana step landed or processing, EVM step landed or pending, action succeeded) unless
  --refund-signature names the finalized transaction that returned the USDC to the member's wallet. confirmed is
  refused when the chain shows it failed or never landed. Both need the source signature on the row. --override books
  it anyway. The operator, reason and chain facts are logged as funding.withdrawal.support_resolved.

usage: monacoctl funding deposit rescan <wallet> [--from <signature>]
  scans the wallet again from its first sight, or after <signature>; credits stay idempotent`

const bounceListLimit = 200

func toolFunding(env toolEnv) tool { return fundingTool(env.environ, clock.Real{}) }

type withdrawalResolver interface {
	Handle(context.Context, fundingapp.ResolveEVMWithdrawal) (fundingapp.EVMWithdrawalResolution, error)
	ListSupport(context.Context) ([]fundingapp.EVMSupportWithdrawal, error)
}

type withdrawalResolvers func(module.Deps) withdrawalResolver

type bounceAction struct {
	args int
	run  func(ctx context.Context, b bounceOps, id uuid.UUID, args []string, operator string) error
}

type bounceOps interface {
	SetReturnAddress(ctx context.Context, id uuid.UUID, address, operator string) error
	Retry(ctx context.Context, id uuid.UUID, operator string) error
	Hold(ctx context.Context, id uuid.UUID, operator string) error
}

func bounceActions() map[string]bounceAction {
	return map[string]bounceAction{
		"set-return-address": {
			args: 1,
			run: func(ctx context.Context, b bounceOps, id uuid.UUID, a []string, op string) error {
				return b.SetReturnAddress(ctx, id, a[0], op)
			},
		},
		"retry": {run: func(ctx context.Context, b bounceOps, id uuid.UUID, _ []string, op string) error {
			return b.Retry(ctx, id, op)
		}},
		"hold": {run: func(ctx context.Context, b bounceOps, id uuid.UUID, _ []string, op string) error {
			return b.Hold(ctx, id, op)
		}},
	}
}

func operatorOf(environ []string) string {
	for _, kv := range environ {
		if name, ok := strings.CutPrefix(kv, "USER="); ok && name != "" {
			return name
		}
	}
	return "unknown"
}

func fundingTool(environ []string, clk clock.Clock) tool {
	return fundingToolWith(environ, clk, func(deps module.Deps) withdrawalResolver {
		return funding.New(deps).EVMWithdrawalResolver()
	})
}

func fundingToolWith(environ []string, clk clock.Clock, resolvers withdrawalResolvers) tool {
	bounce := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if firstArg(args) == "list" {
			return listBounces(cfg, args[1:], stdout, stderr)
		}
		action, ok := bounceActions()[firstArg(args)]
		if !ok || len(args) != 2+action.args {
			_, _ = fmt.Fprintln(stderr, fundingUsage)
			return 2
		}
		id, err := uuid.Parse(args[1])
		if err != nil {
			_, _ = fmt.Fprintln(stderr, fundingUsage)
			return 2
		}
		ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		deps := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clk)}
		if err := action.run(ctx, funding.New(deps).Bouncer(), id, args[2:], operatorOf(environ)); err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintf(stdout, "%s %s\n", args[0], id)
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		if slices.ContainsFunc(args, func(a string) bool { return a == "-h" || a == "--help" || a == "help" }) {
			_, _ = fmt.Fprintln(stdout, fundingUsage)
			return 0
		}
		return run(
			map[string]command{
				"bounce": bounce,
				"withdrawal": func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
					return withdrawalCommand(cfg, clk, resolvers, operatorOf(environ), args, stdout, stderr)
				},
				"deposit": func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
					return depositCommand(cfg, clk, args, stdout, stderr)
				},
			},
			nil,
			environ,
			args,
			stdout,
			stderr,
		)
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func listBounces(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	statuses := args
	if len(statuses) == 0 {
		statuses = fundingapp.OpenBounceStatuses()
	}
	for _, status := range statuses {
		if !slices.Contains(fundingapp.ExternalDepositStatuses(), status) {
			_, _ = fmt.Fprintf(stderr, "unknown status %q\n%s\n", status, fundingUsage)
			return 2
		}
	}
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	rows, err := fundingapp.BounceQueue(ctx, pool, statuses, bounceListLimit)
	if err != nil {
		return fail(stderr, err)
	}
	for _, r := range rows {
		returnAddress := r.ReturnAddress
		if returnAddress == "" {
			returnAddress = "-"
		}
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", r.ExternalDepositID, r.CabalID.UUID(), r.Status,
			r.Amount, r.Mint, returnAddress, r.Attempts, r.Since.UTC().Format(time.RFC3339))
	}
	return 0
}

func depositCommand(cfg config.Config, clk clock.Clock, args []string, stdout, stderr io.Writer) int {
	if firstArg(args) != "rescan" || len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, fundingUsage)
		return 2
	}
	var from chain.Signature
	switch len(args) {
	case 2:
	case 4:
		if args[2] != "--from" {
			_, _ = fmt.Fprintln(stderr, fundingUsage)
			return 2
		}
		from = chain.Signature(args[3])
	default:
		_, _ = fmt.Fprintln(stderr, fundingUsage)
		return 2
	}
	wallet, err := chain.ParseAddress(args[1])
	if err != nil {
		return fail(stderr, err)
	}
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	rescan := fundingapp.DepositRescan{UoW: db.New(pool, ids.Real{}, clk)}
	accounts, err := rescan.Rescan(ctx, wallet, from)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "rescan %s %d\n", wallet, accounts)
	return 0
}

func withdrawalCommand(
	cfg config.Config, clk clock.Clock, resolvers withdrawalResolvers, operator string, args []string,
	stdout, stderr io.Writer,
) int {
	sub := firstArg(args)
	if (sub != "resolve" || len(args) < 2) && (sub != "list-support" || len(args) != 1) {
		_, _ = fmt.Fprintln(stderr, fundingUsage)
		return 2
	}
	var cmd fundingapp.ResolveEVMWithdrawal
	if sub == "resolve" {
		parsed, ok := parseResolve(args[1:], operator)
		if !ok {
			_, _ = fmt.Fprintln(stderr, fundingUsage)
			return 2
		}
		cmd = parsed
	}
	ctx := auth.WithActor(context.Background(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	deps := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clk)}
	resolver := resolvers(deps)
	if sub == "list-support" {
		return listSupport(ctx, resolver, stdout, stderr)
	}
	resolved, err := resolver.Handle(ctx, cmd)
	if err != nil {
		return fail(stderr, err)
	}
	facts, _ := json.Marshal(resolved.Facts)
	_, _ = fmt.Fprintf(stdout, "resolve %s %s %s\n", resolved.ID, resolved.Status, facts)
	return 0
}

func parseResolve(args []string, operator string) (fundingapp.ResolveEVMWithdrawal, bool) {
	id, err := uuid.Parse(args[0])
	fs := flag.NewFlagSet("funding withdrawal resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.String("to", "", "")
	reason := fs.String("reason", "", "")
	refund := fs.String("refund-signature", "", "")
	override := fs.Bool("override", false, "")
	if err != nil || fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *reason == "" ||
		(*to != string(domain.WithdrawalConfirmed) && *to != string(domain.WithdrawalFailed)) {
		return fundingapp.ResolveEVMWithdrawal{}, false
	}
	return fundingapp.ResolveEVMWithdrawal{
		ID: id, To: domain.WithdrawalStatus(*to), Reason: *reason, Actor: "operator:" + operator,
		Override: *override, RefundSignature: chain.Signature(*refund),
	}, true
}

func listSupport(ctx context.Context, resolver withdrawalResolver, stdout, stderr io.Writer) int {
	rows, err := resolver.ListSupport(ctx)
	for _, r := range rows {
		signature := string(r.SourceSignature)
		if signature == "" {
			signature = "-"
		}
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.UserID, r.Amount,
			r.Age.Round(time.Minute), signature, r.ActionID)
	}
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}
