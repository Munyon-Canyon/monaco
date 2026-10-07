package app

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const authHistoryLimit = 200

type CabalName struct {
	ID   ids.CabalID
	Name string
}

type UserView struct {
	Card    identityport.UserCard
	Wallet  string
	History []events.UserAuthStateChanged
	Cabals  []CabalName
	Shares  []treasuryport.Share
	Txns    []TxnHeader
	Actions []sqlc.AdminAction
}

type UserLookup struct {
	Users   UserReader
	Wallets WalletReader
	Cabals  CabalIndex
	Shares  ShareLists
	Txns    TxnLists
	Events  EventHistory
	Actions ActionLog
}

func (l UserLookup) ByID(ctx context.Context, id ids.UserID) (UserView, error) {
	cards, err := l.Users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return UserView{}, err
	}
	card, ok := cards[id]
	if !ok {
		return UserView{}, errs.New(errs.CodeUserNotFound, "admin.UserLookup.ByID")
	}
	return l.view(ctx, card)
}

func (l UserLookup) ByHandle(ctx context.Context, handle string) (UserView, error) {
	card, err := l.Users.UserByHandle(ctx, handle)
	if err != nil {
		return UserView{}, err
	}
	return l.view(ctx, card)
}

func (l UserLookup) view(ctx context.Context, card identityport.UserCard) (UserView, error) {
	view := UserView{Card: card}
	steps := []func(context.Context, *UserView) error{
		l.loadWallet, l.loadHistory, l.loadCabals, l.loadShares, l.loadTxns, l.loadActions,
	}
	for _, step := range steps {
		if err := step(ctx, &view); err != nil {
			return UserView{}, err
		}
	}
	return view, nil
}

func (l UserLookup) loadWallet(ctx context.Context, v *UserView) error {
	wallet, err := l.Wallets.MemberWallet(ctx, v.Card.ID)
	switch {
	case errs.CodeOf(err) == errs.CodeUserNotFound:
		return nil
	case err != nil:
		return err
	}
	v.Wallet = ShortAddress(string(wallet.Address))
	return nil
}

func (l UserLookup) loadHistory(ctx context.Context, v *UserView) error {
	const op = "admin.UserLookup.loadHistory"
	rows, err := l.Events.EventsByAggregate(
		ctx, "user", v.Card.ID.UUID(), []string{string(events.TypeUserAuthStateChanged)}, authHistoryLimit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var change events.UserAuthStateChanged
		if err := json.Unmarshal(row.Payload, &change); err != nil {
			return errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("event_id", row.ID.String()))
		}
		v.History = append(v.History, change)
	}
	return nil
}

func (l UserLookup) loadCabals(ctx context.Context, v *UserView) error {
	cabalIDs, err := l.Cabals.CabalsOf(ctx, v.Card.ID)
	if err != nil || len(cabalIDs) == 0 {
		return err
	}
	views, err := l.Cabals.Cabals(ctx, cabalIDs)
	if err != nil {
		return err
	}
	for _, id := range cabalIDs {
		if cabal, ok := views[id]; ok {
			v.Cabals = append(v.Cabals, CabalName{ID: id, Name: cabal.Name})
		}
	}
	return nil
}

func (l UserLookup) loadShares(ctx context.Context, v *UserView) (err error) {
	v.Shares, err = l.Shares.UserShares(ctx, v.Card.ID)
	return err
}

func (l UserLookup) loadTxns(ctx context.Context, v *UserView) error {
	txns, err := l.Txns.UserTxns(ctx, v.Card.ID, nil, recentLimit)
	v.Txns = txnHeaders(txns)
	return err
}

func (l UserLookup) loadActions(ctx context.Context, v *UserView) (err error) {
	v.Actions, err = recentActions(ctx, l.Actions, "user", v.Card.ID.String())
	return err
}
