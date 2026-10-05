package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const livePriceAge = 5 * time.Minute

type DetectExternalDeposit struct {
	Signature chain.Signature
	CabalID   ids.CabalID
	Treasury  chain.SolanaAddress
	Source    domain.DepositSource
}

type DetectResult struct {
	Recorded          bool
	Verdict           domain.Verdict
	ExternalDepositID uuid.UUID
}

func (r DetectResult) Outcome() errs.Code {
	if !r.Recorded {
		return ""
	}
	return r.Verdict.Code()
}

type WatchChain interface {
	InboundTransfers(ctx context.Context, sig chain.Signature, owner chain.SolanaAddress) ([]solana.Transfer, error)
}

type WatchAssets interface {
	AssetByMint(ctx context.Context, mint market.Mint) (market.Asset, error)
}

type WatchPrices interface {
	LatestPrices(ctx context.Context) (map[market.AssetID]market.Price, error)
}

type DetectDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	IDs     ids.Generator
	Clock   clock.Clock
	Hints   HintPublisher
	Chain   WatchChain
	Owners  []fundingport.SignatureOwner
	Assets  WatchAssets
	Prices  WatchPrices
	Wallets identityport.WalletReader
	USDC    chain.SolanaAddress
}

type DetectExternalDepositHandler struct{ d DetectDeps }

func NewDetectExternalDepositHandler(d DetectDeps) *DetectExternalDepositHandler {
	return &DetectExternalDepositHandler{d: d}
}

type inboundTransfer struct {
	transfer solana.Transfer
	asset    *market.Asset
	verdict  domain.Verdict
}

func (h *DetectExternalDepositHandler) Handle(ctx context.Context, cmd DetectExternalDeposit) (DetectResult, error) {
	const op = "funding.DetectExternalDeposit"
	seen, err := sqlc.New(h.d.Reads).ExternalDepositSeen(ctx, string(cmd.Signature))
	if err != nil {
		return DetectResult{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if seen {
		return DetectResult{}, nil
	}
	owned, err := h.owned(ctx, cmd.Signature)
	if err != nil {
		return DetectResult{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if owned {
		observability.Info(ctx, observability.FundingWatchOwnTransfer,
			slog.String("cabal_id", cmd.CabalID.String()), slog.String("tx", string(cmd.Signature)))
		return DetectResult{Verdict: domain.VerdictOwn}, nil
	}
	transfers, err := h.d.Chain.InboundTransfers(ctx, cmd.Signature, cmd.Treasury)
	if err != nil {
		return DetectResult{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	in, ok, err := h.worst(ctx, transfers)
	if err != nil || !ok {
		return DetectResult{}, err
	}
	if in.verdict != domain.VerdictDetected {
		return h.ignore(ctx, cmd, in)
	}
	return h.detect(ctx, cmd, in)
}

func (h *DetectExternalDepositHandler) owned(ctx context.Context, sig chain.Signature) (bool, error) {
	for _, owner := range h.d.Owners {
		owned, err := owner.OwnsSignature(ctx, sig)
		if err != nil || owned {
			return owned, err
		}
	}
	return false, nil
}

func (h *DetectExternalDepositHandler) worst(
	ctx context.Context, transfers []solana.Transfer,
) (inboundTransfer, bool, error) {
	var out inboundTransfer
	found := false
	for _, t := range transfers {
		in, err := h.classify(ctx, t)
		if err != nil {
			return inboundTransfer{}, false, err
		}
		if !found || in.verdict > out.verdict {
			out, found = in, true
		}
	}
	return out, found, nil
}

func (h *DetectExternalDepositHandler) classify(ctx context.Context, t solana.Transfer) (inboundTransfer, error) {
	const op = "funding.DetectExternalDeposit.classify"
	in := inboundTransfer{transfer: t}
	if t.Mint.Address == h.d.USDC {
		value := money.MicrosFromUint64(t.Net.Uint64())
		in.verdict = domain.Classify(domain.Inbound{Known: true, Value: &value})
		return in, nil
	}
	asset, known, err := h.listed(ctx, t.Mint.Address)
	if err != nil {
		return inboundTransfer{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if !known {
		in.verdict = domain.Classify(domain.Inbound{})
		return in, nil
	}
	in.asset = &asset
	value, priced, err := h.stockValue(ctx, asset, t.Net)
	if err != nil {
		return inboundTransfer{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	facts := domain.Inbound{Known: true}
	if priced {
		facts.Value = &value
	}
	in.verdict = domain.Classify(facts)
	return in, nil
}

func (h *DetectExternalDepositHandler) listed(
	ctx context.Context, addr chain.SolanaAddress,
) (market.Asset, bool, error) {
	mint, err := market.ParseMint(string(addr))
	if err != nil {
		return market.Asset{}, false, err
	}
	asset, err := h.d.Assets.AssetByMint(ctx, mint)
	if errs.CodeOf(err) == errs.CodeAssetNotFound {
		return market.Asset{}, false, nil
	}
	return asset, err == nil, err
}

func (h *DetectExternalDepositHandler) stockValue(
	ctx context.Context, asset market.Asset, units money.BaseUnits,
) (money.Micros, bool, error) {
	prices, err := h.d.Prices.LatestPrices(ctx)
	if err != nil {
		return money.Micros{}, false, err
	}
	now := h.d.Clock.Now()
	price, ok := prices[asset.ID]
	m := asset.UIMultiplierAt(now)
	if !ok || now.Sub(price.ObservedAt) > livePriceAge || m.Num <= 0 || m.Den <= 0 {
		return money.Micros{}, false, nil
	}
	value, err := domain.StockValue(units, price.Micros, uint64(m.Num), uint64(m.Den))
	return value, err == nil, err
}

func (h *DetectExternalDepositHandler) row(
	cmd DetectExternalDeposit, in inboundTransfer, id uuid.UUID, now time.Time,
) sqlc.InsertExternalDepositParams {
	p := sqlc.InsertExternalDepositParams{
		ID: id, Signature: string(cmd.Signature), CabalID: cmd.CabalID.UUID(), Sender: string(in.transfer.From),
		Mint: string(in.transfer.Mint.Address), Amount: in.transfer.Net.String(), Source: string(cmd.Source),
		Status: string(in.verdict.Status()), DetectedAt: now,
	}
	if in.asset != nil {
		p.AssetID = in.asset.ID.UUID()
	}
	if in.verdict != domain.VerdictDetected {
		p.ResolvedAt = now
	}
	return p
}

func (h *DetectExternalDepositHandler) ignore(
	ctx context.Context, cmd DetectExternalDeposit, in inboundTransfer,
) (DetectResult, error) {
	id, now := h.d.IDs.NewV7(), h.d.Clock.Now()
	var inserted int64
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		inserted, err = sqlc.New(tx.Queries()).InsertExternalDeposit(ctx, h.row(cmd, in, id, now))
		return err
	})
	if err != nil || inserted == 0 {
		return DetectResult{}, err
	}
	observability.Info(ctx, observability.FundingWatchIgnored,
		slog.String("cabal_id", cmd.CabalID.String()), slog.String("tx", string(cmd.Signature)),
		slog.String("mint", string(in.transfer.Mint.Address)), slog.String("amount", in.transfer.Net.String()),
		slog.String("outcome", string(in.verdict.Code())))
	return DetectResult{Recorded: true, Verdict: in.verdict, ExternalDepositID: id}, nil
}

func (h *DetectExternalDepositHandler) detect(
	ctx context.Context, cmd DetectExternalDeposit, in inboundTransfer,
) (DetectResult, error) {
	sender, err := h.senderUser(ctx, in.transfer.From)
	if err != nil {
		return DetectResult{}, errs.Wrap(err, errs.CodeOf(err), "funding.DetectExternalDeposit.detect")
	}
	id, now := h.d.IDs.NewV7(), h.d.Clock.Now()
	var inserted int64
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		inserted, err = sqlc.New(tx.Queries()).InsertExternalDeposit(ctx, h.row(cmd, in, id, now))
		if err != nil || inserted == 0 {
			return err
		}
		pause := newPause{
			id: h.d.IDs.NewV7(), scope: pauseScope{cabal: &cmd.CabalID}, reason: domain.PauseReasonExternalDeposit,
			externalDeposit: id, at: now,
		}
		if err := writePause(ctx, tx, pause, h.d.Hints); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Info(ctx, observability.FundingWatchDetected,
				slog.String("external_deposit_id", id.String()), slog.String("cabal_id", cmd.CabalID.String()),
				slog.String("mint", string(in.transfer.Mint.Address)), slog.String("amount", in.transfer.Net.String()),
				slog.String("status_before", "none"), slog.String("status_after", string(domain.ExternalDetected)))
		})
		return tx.Events.Append(ctx, h.event(cmd, in, id, sender))
	})
	if err != nil || inserted == 0 {
		return DetectResult{}, err
	}
	return DetectResult{Recorded: true, Verdict: domain.VerdictDetected, ExternalDepositID: id}, nil
}

func (h *DetectExternalDepositHandler) event(
	cmd DetectExternalDeposit, in inboundTransfer, id, sender uuid.UUID,
) events.CabalExternalDepositDetected {
	e := events.CabalExternalDepositDetected{
		V: 1, ExternalDepositID: id, CabalID: cmd.CabalID.UUID(), Signature: cmd.Signature,
		Sender: in.transfer.From, Mint: in.transfer.Mint.Address, Amount: in.transfer.Net.Uint64(),
	}
	if sender != uuid.Nil {
		e.SenderUserID = &sender
	}
	if in.asset != nil {
		asset := in.asset.ID.UUID()
		e.AssetID = &asset
	}
	return e
}

func (h *DetectExternalDepositHandler) senderUser(ctx context.Context, from chain.SolanaAddress) (uuid.UUID, error) {
	var after ids.UserID
	for {
		page, err := h.d.Wallets.MemberWallets(ctx, after, identityport.MaxWalletPage)
		if err != nil {
			return uuid.Nil, err
		}
		for _, w := range page {
			if w.Address == from {
				return w.UserID.UUID(), nil
			}
		}
		if len(page) < identityport.MaxWalletPage {
			return uuid.Nil, nil
		}
		after = page[len(page)-1].UserID
	}
}
