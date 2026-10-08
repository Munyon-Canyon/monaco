package app

import (
	"context"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type MemberDetail struct {
	View       cabalport.MemberView
	Handle     string
	ShareUnits money.SharesUnits
}

type HoldingDetail struct {
	Mint   string
	Symbol string
	Units  money.BaseUnits
}

type Valuation struct {
	Pot         money.Micros
	NavPerShare money.Micros
	At          time.Time
}

type CabalView struct {
	Cabal    cabalport.CabalView
	Rules    cabalport.Rules
	Treasury cabalport.TreasuryWallet
	Members  []MemberDetail
	Holdings []HoldingDetail
	Value    *Valuation
	Pauses   []fundingport.OpenPause
	Txns     []TxnHeader
	Actions  []sqlc.AdminAction
}

type CabalLookup struct {
	Cabals   CabalReader
	Details  CabalDetails
	Users    UserReader
	Shares   ShareLists
	Holdings Holdings
	Assets   Assets
	Values   Valuations
	Pauses   PauseReader
	Txns     TxnLists
	Actions  ActionLog
}

func (l CabalLookup) ByID(ctx context.Context, id ids.CabalID) (CabalView, error) {
	cabal, err := l.Cabals.Cabal(ctx, id)
	if err != nil {
		return CabalView{}, err
	}
	view := CabalView{Cabal: cabal}
	steps := []func(context.Context, *CabalView) error{
		l.loadRules,
		l.loadTreasury,
		l.loadMembers,
		l.loadHoldings,
		l.loadValue,
		l.loadPauses,
		l.loadTxns,
		l.loadActions,
	}
	for _, step := range steps {
		if err := step(ctx, &view); err != nil {
			return CabalView{}, err
		}
	}
	return view, nil
}

func (l CabalLookup) loadRules(ctx context.Context, v *CabalView) (err error) {
	v.Rules, err = l.Details.Rules(ctx, v.Cabal.ID)
	return err
}

func (l CabalLookup) loadTreasury(ctx context.Context, v *CabalView) (err error) {
	v.Treasury, err = l.Details.TreasuryWallet(ctx, v.Cabal.ID)
	return err
}

func (l CabalLookup) loadMembers(ctx context.Context, v *CabalView) error {
	members, err := l.Details.Members(ctx, v.Cabal.ID)
	if err != nil {
		return err
	}
	shares, err := l.Shares.CabalShares(ctx, v.Cabal.ID)
	if err != nil {
		return err
	}
	handles, err := l.handles(ctx, members)
	if err != nil {
		return err
	}
	units := make(map[ids.UserID]money.SharesUnits, len(shares))
	for _, s := range shares {
		units[s.UserID] = s.ShareUnits
	}
	for _, m := range members {
		v.Members = append(v.Members, MemberDetail{View: m, Handle: handles[m.UserID], ShareUnits: units[m.UserID]})
	}
	return nil
}

func (l CabalLookup) handles(ctx context.Context, members []cabalport.MemberView) (map[ids.UserID]string, error) {
	userIDs := make([]ids.UserID, len(members))
	for i, m := range members {
		userIDs[i] = m.UserID
	}
	out := make(map[ids.UserID]string, len(members))
	for chunk := range slices.Chunk(userIDs, identityport.MaxUsersByID) {
		cards, err := l.Users.UsersByID(ctx, chunk)
		if err != nil {
			return nil, err
		}
		for id, card := range cards {
			out[id] = card.Handle
		}
	}
	return out, nil
}

func (l CabalLookup) loadHoldings(ctx context.Context, v *CabalView) error {
	positions, err := l.Holdings.CabalHoldings(ctx, v.Cabal.ID)
	if err != nil {
		return err
	}
	assets, err := l.Assets.ListAll(ctx)
	if err != nil {
		return err
	}
	symbols := make(map[string]string, len(assets))
	for _, a := range assets {
		symbols[a.Mint.String()] = a.Symbol
	}
	for _, p := range positions {
		v.Holdings = append(v.Holdings, HoldingDetail{
			Mint: string(p.Mint), Symbol: symbols[string(p.Mint)], Units: p.Units,
		})
	}
	return nil
}

func (l CabalLookup) loadValue(ctx context.Context, v *CabalView) error {
	values, err := l.Values.LatestCabalValues(ctx)
	if i := slices.IndexFunc(values, func(c rankingport.CabalValue) bool { return c.CabalID == v.Cabal.ID }); i >= 0 {
		v.Value = &Valuation{Pot: values[i].Value, NavPerShare: values[i].NavPerShare, At: values[i].At}
	}
	return err
}

func (l CabalLookup) loadPauses(ctx context.Context, v *CabalView) error {
	pause, err := l.Pauses.IsPaused(ctx, v.Cabal.ID)
	v.Pauses = pause.Open
	return err
}

func (l CabalLookup) loadTxns(ctx context.Context, v *CabalView) error {
	txns, err := l.Txns.CabalTxns(ctx, v.Cabal.ID, nil, recentLimit)
	v.Txns = txnHeaders(txns)
	return err
}

func (l CabalLookup) loadActions(ctx context.Context, v *CabalView) (err error) {
	v.Actions, err = recentActions(ctx, l.Actions, "cabal", v.Cabal.ID.String())
	return err
}
