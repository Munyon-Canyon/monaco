package trading_test

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/swaptx"
)

type tableFake struct {
	tables platform.LookupTables
	err    error
	reads  atomic.Int64
}

func (f *tableFake) LookupTables(context.Context, []platform.SolanaAddress) (platform.LookupTables, error) {
	f.reads.Add(1)
	return f.tables, f.err
}

func intentSpec(req app.SwapRequest) swaptx.Spec {
	return swaptx.Spec{
		Payer: chainfake.RelayerAddress(), Wallet: req.TreasuryWallet.Address, In: req.InMint, Out: req.OutMint,
		InAmount: req.InAmount, QuotedOut: req.QuoteOutAmount, SlippageBps: swaptx.Bps(req.SlippageBps),
	}
}

func (e *layerEnv) offer(tx []byte) {
	e.jup.SetRawOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()), jupiter.Order{
		RequestID: "req-1", Transaction: tx,
	})
}

func (e *layerEnv) countSigns() *atomic.Int64 {
	var signs atomic.Int64
	e.signer = signerFunc(
		func(ctx context.Context, wallet string, unsigned []byte) ([]byte, platform.Signature, error) {
			signs.Add(1)
			return e.chainSigner().Sign(ctx, wallet, unsigned)
		},
	)
	return &signs
}

func (e *layerEnv) assertRefusedBeforeSigning(t *testing.T, req app.SwapRequest, signs *atomic.Int64) {
	t.Helper()
	got, err := e.run(t, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed || got.FailureCode != domain.FailureNeverSubmitted || signs.Load() != 0 ||
		len(e.jup.Sent("req-1")) != 0 {
		t.Fatalf("view = %+v after %d signs and %d executes, want failed never_submitted and nothing signed or sent",
			got, signs.Load(), len(e.jup.Sent("req-1")))
	}
	if events := e.events(t, got.ID.UUID()); len(events) == 0 || events[len(events)-1] != "trade.failed" {
		t.Fatalf("events = %v, want a closing trade.failed and no trade.submitted", events)
	}
}

func TestSwapLayer_HonestBytesWithALookupTableAreSigned(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	req := e.request(e.source())
	tx := swaptx.HonestTx(intentSpec(req))
	tx.Table, tx.Tabled = swaptx.Address("lookup"), []platform.SolanaAddress{
		req.OutMint.Address, req.InMint.Address, swaptx.Address("event-authority"),
	}
	tables := &tableFake{tables: tx.Tables()}
	e.tables = tables
	e.signer = chain.NewSigner(e.privy, chainfake.RelayerWith(chainfake.Tables{Loaded: tx.Tables()}))
	e.offer(tx.Bytes())
	e.jup.SetExecute("req-1", jupiter.ExecuteResult{Status: jupiter.StatusPending})

	got, err := e.run(t, req)
	if err != nil || got.Status != domain.StatusSubmitted || tables.reads.Load() != 1 {
		t.Fatalf(
			"view = %+v, %v after %d table reads, want submitted with one table read",
			got,
			err,
			tables.reads.Load(),
		)
	}
}

func TestSwapLayer_MaliciousBytesWithValidLookingMetadataAreRefusedBeforeSigning(t *testing.T) {
	t.Parallel()
	attacker := swaptx.Address("attacker")
	build := func(req app.SwapRequest, mutate func(*swaptx.Tx, *swaptx.Spec)) swaptx.Tx {
		spec := intentSpec(req)
		var tx swaptx.Tx
		mutate(&tx, &spec)
		honest := swaptx.HonestTx(spec)
		if tx.Ixs != nil {
			honest.Ixs = tx.Ixs
		}
		return honest
	}
	extra := func(ix swaptx.Ix) func(*swaptx.Tx, *swaptx.Spec) {
		return func(tx *swaptx.Tx, spec *swaptx.Spec) {
			ixs := swaptx.HonestIxs(*spec)
			tx.Ixs = append(append([]swaptx.Ix(nil), ixs[:len(ixs)-1]...), ix, ixs[len(ixs)-1])
		}
	}
	token := func(tag byte, accounts ...platform.SolanaAddress) swaptx.Ix {
		return swaptx.Ix{Program: platform.SPLProgram, Accounts: accounts, Data: []byte{tag, 0, 0, 0, 0, 0, 0, 0, 0}}
	}
	cases := map[string]func(*swaptx.Tx, *swaptx.Spec){
		"wrong mint": func(_ *swaptx.Tx, s *swaptx.Spec) {
			s.Out = platform.Mint{Address: swaptx.Address("worthless"), Decimals: 6}
		},
		"extra transfer": extra(swaptx.Ix{
			Program:  platform.SystemProgram,
			Accounts: []platform.SolanaAddress{chainfake.WalletAddress(treasuryWallet), attacker},
			Data:     binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint32(nil, 2), 1),
		}),
		"extra token transfer": extra(token(
			3,
			swaptx.ATA(
				chainfake.WalletAddress(treasuryWallet),
				usdcToken().Address,
			),
			swaptx.ATA(attacker, usdcToken().Address),
			chainfake.WalletAddress(treasuryWallet),
		)),
		"authority change": extra(token(6, swaptx.ATA(chainfake.WalletAddress(treasuryWallet), usdcToken().Address),
			chainfake.WalletAddress(treasuryWallet))),
		"close account": extra(
			token(9, swaptx.ATA(chainfake.WalletAddress(treasuryWallet), usdcToken().Address), attacker,
				chainfake.WalletAddress(treasuryWallet)),
		),
		"wrong recipient": func(tx *swaptx.Tx, s *swaptx.Spec) {
			route := swaptx.SharedRoute(*s)
			route.Accounts = append([]platform.SolanaAddress(nil), route.Accounts...)
			route.Accounts[6] = swaptx.ATA(attacker, s.Out.Address)
			ixs := swaptx.HonestIxs(*s)
			tx.Ixs = append(append([]swaptx.Ix(nil), ixs[:len(ixs)-1]...), route)
		},
		"amount above the ceiling": func(_ *swaptx.Tx, s *swaptx.Spec) { s.InAmount++ },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newLayerEnv(t)
			signs := e.countSigns()
			req := e.request(e.source())
			e.offer(build(req, mutate).Bytes())
			e.assertRefusedBeforeSigning(t, req, signs)
		})
	}
}

func TestSwapLayer_ALookupTableCannotHideAWrongRecipient(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	signs := e.countSigns()
	req := e.request(e.source())
	spec := intentSpec(req)
	route := swaptx.SharedRoute(spec)
	route.Accounts = append([]platform.SolanaAddress(nil), route.Accounts...)
	route.Accounts[6] = swaptx.ATA(swaptx.Address("attacker"), spec.Out.Address)
	tx := swaptx.HonestTx(spec)
	tx.Ixs = append(tx.Ixs[:len(tx.Ixs)-1:len(tx.Ixs)-1], route)
	tx.Table, tx.Tabled = swaptx.Address("lookup"), []platform.SolanaAddress{route.Accounts[6], req.OutMint.Address}
	e.tables = &tableFake{tables: tx.Tables()}
	e.offer(tx.Bytes())

	e.assertRefusedBeforeSigning(t, req, signs)
}

func TestSwapLayer_ATableThatCannotBeReadFailsTheSwapBeforeSigning(t *testing.T) {
	t.Parallel()
	for name, tables := range map[string]app.LookupTables{
		"rpc down":  &tableFake{err: errs.New(errs.CodeRPCUnavailable, "test")},
		"no reader": nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newLayerEnv(t)
			signs := e.countSigns()
			req := e.request(e.source())
			tx := swaptx.HonestTx(intentSpec(req))
			tx.Table, tx.Tabled = swaptx.Address("lookup"), []platform.SolanaAddress{req.OutMint.Address}
			e.tables = tables
			e.offer(tx.Bytes())
			e.passTime(t)
			got, err := e.run(t, req)
			if signs.Load() != 0 || len(e.jup.Sent("req-1")) != 0 {
				t.Fatalf(
					"view %+v, %v: %d signs, %d executes, want nothing signed or sent",
					got,
					err,
					signs.Load(),
					len(e.jup.Sent("req-1")),
				)
			}
			if name != "rpc down" {
				return
			}
			swaps := e.swapIDs(t, req.Source.ID)
			if err == nil || len(swaps) != 1 ||
				e.payload(t, swaps[0], "trade.failed")["failure_code"] != string(domain.FailureNeverSubmitted) {
				t.Fatalf("err = %v, swaps = %v, want the swap failed never_submitted after the retries", err, swaps)
			}
		})
	}
}
