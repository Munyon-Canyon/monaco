package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	tradingadapters "github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	tradingchain "github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	treasuryapp "github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	treasurydomain "github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type cashAdapters struct {
	Wallets   tradingapp.TreasuryEVM
	Cash      tradingapp.EVMCash
	Transfers tradingapp.EVMTransfers
	WriteOffs tradingapp.EVMCashWriteOffs
	Chain     tradingapp.ChainReader

	Receipts tradingapp.RoutedReceiptReader
}

type cashPorts func(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk clock.Clock) cashAdapters

func liveCashPorts(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk clock.Clock) cashAdapters {
	deps := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: uow, HTTPClient: httpclient.New}
	wallets := cabal.New(deps).TreasuryEVMWallets()
	return cashAdapters{
		Wallets:   wallets,
		Cash:      tradingadapters.NewEVMCash(tradingadapters.EVMRPC(deps)),
		Transfers: tradingadapters.NewEVMArrivals(tradingadapters.EVMRPC(deps), int64(cfg.EVM.LogsMaxBlocks), clk),
		WriteOffs: cashWriteOffs{ledger: treasuryapp.NewLedger(chain.SolanaAddress(cfg.Solana.USDCMint), clk)},
		Chain:     tradingchain.NewReader(solana.New(cfg, clk)),
		Receipts: tradingapp.RoutedReceipts{
			Reads: pool, Listings: market.New(deps).Catalog(), Wallets: wallets,
			Venue: tradingadapters.NewEVMVenue(nil, "", tradingadapters.EVMRPC(deps), clk),
		},
	}
}

type cashWriteOffs struct{ ledger treasuryapp.Ledger }

func (w cashWriteOffs) WriteOff(ctx context.Context, tx db.Tx, off tradingapp.EVMCashWriteOff) error {
	return w.ledger.WriteOffEVMCash(ctx, tx, treasurydomain.CashWriteOff{
		SwapID: off.SwapID.UUID(), CabalID: off.CabalID, Network: off.Network, Amount: off.Amount,
	})
}

func parseReturnEVMCash(environ []string, args []string) (tradingapp.ReturnEVMCash, bool, bool) {
	fs := flag.NewFlagSet("swaps return-evm-cash", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	swap := fs.String("swap", "", "")
	reason := fs.String("reason", "", "")
	yes := fs.Bool("yes", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *reason == "" {
		return tradingapp.ReturnEVMCash{}, false, false
	}
	id, err := uuid.Parse(*swap)
	if err != nil {
		return tradingapp.ReturnEVMCash{}, false, false
	}
	return tradingapp.ReturnEVMCash{
		SwapID: ids.SwapIDFrom(id), Actor: "operator:" + operatorOf(environ), Reason: *reason,
	}, *yes, true
}

func returnEVMCash(environ []string, clk clock.Clock, stdin io.Reader, ports cashPorts) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		cmd, yes, ok := parseReturnEVMCash(environ, args)
		if !ok {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		uow := db.New(pool, ids.Real{}, clk)
		adapters := ports(cfg, pool, uow, clk)
		handler := tradingapp.NewReturnEVMCashHandler(
			uow,
			pool,
			clk,
			ids.Real{},
			adapters.Wallets,
			adapters.Cash,
			adapters.WriteOffs,
		)
		plan, err := handler.Plan(ctx, cmd.SwapID)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "plan "+evmCashLine(plan))
		if !yes && !confirmed(stdin, stderr, "Return this cash to Solana?") {
			_, _ = fmt.Fprintln(stderr, "monacoctl: cancelled")
			return 1
		}
		queued, err := handler.Handle(ctx, cmd)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "queued "+evmCashLine(queued)+" status=pending")
		return 0
	}
}

func evmCashLine(r tradingapp.EVMCashReturn) string {
	return fmt.Sprintf(
		"swap=%s requested=%s cabal=%s network=%s landed_micros=%s on_chain_micros=%s amount_micros=%s uncovered_micros=%s",
		r.Holder.UUID(),
		r.Requested.UUID(),
		r.CabalID.UUID(),
		r.Network,
		r.Landed,
		r.OnChain,
		r.Amount,
		r.Uncovered,
	)
}
