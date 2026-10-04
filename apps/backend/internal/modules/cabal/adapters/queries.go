package adapters

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Postgres struct {
	q *sqlc.Queries
}

var _ port.Queries = Postgres{}

func NewQueries(db sqlc.DBTX) Postgres { return Postgres{q: sqlc.New(db)} }

func (r Postgres) AllCabals(ctx context.Context) ([]port.CabalView, error) {
	const op = "cabal.AllCabals"
	rows, err := r.q.AllCabals(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	cabals := make([]port.CabalView, 0, len(rows))
	for _, row := range rows {
		view, err := viewOf(op, sqlc.FindCabalRow(row))
		if err != nil {
			return nil, err
		}
		cabals = append(cabals, view)
	}
	return cabals, nil
}

func (r Postgres) Cabal(ctx context.Context, id ids.CabalID) (port.CabalView, error) {
	const op = "cabal.Cabal"
	row, err := r.find(ctx, op, id)
	if err != nil {
		return port.CabalView{}, err
	}
	return viewOf(op, row)
}

func (r Postgres) Cabals(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]port.CabalView, error) {
	const op = "cabal.Cabals"
	raw := make([]uuid.UUID, len(cabalIDs))
	for i, id := range cabalIDs {
		raw[i] = id.UUID()
	}
	rows, err := r.q.ListCabals(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	found := make(map[ids.CabalID]port.CabalView, len(rows))
	for _, row := range rows {
		view, err := viewOf(op, sqlc.FindCabalRow(row))
		if err != nil {
			return nil, err
		}
		found[view.ID] = view
	}
	return found, nil
}

func (r Postgres) IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error) {
	const op = "cabal.IsMember"
	_, err := r.q.FindMember(ctx, sqlc.FindMemberParams{CabalID: id.UUID(), UserID: user.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, errs.Wrap(err, errs.CodeInternal, op)
	}
	return true, nil
}

func (r Postgres) Member(ctx context.Context, id ids.CabalID, user ids.UserID) (port.MemberView, error) {
	const op = "cabal.Member"
	row, err := r.q.FindMember(ctx, sqlc.FindMemberParams{CabalID: id.UUID(), UserID: user.UUID()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.MemberView{}, errs.New(errs.CodeNotCabalMember, op)
	case err != nil:
		return port.MemberView{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return memberOf(user, row.Role, row.CanVote, row.JoinedAt), nil
}

func (r Postgres) Members(ctx context.Context, id ids.CabalID) ([]port.MemberView, error) {
	const op = "cabal.Members"
	rows, err := r.q.ListMembers(ctx, id.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	members := make([]port.MemberView, 0, len(rows))
	for _, row := range rows {
		user, err := userID(op, row.UserID)
		if err != nil {
			return nil, err
		}
		members = append(members, memberOf(user, row.Role, row.CanVote, row.JoinedAt))
	}
	return members, nil
}

func (r Postgres) MembersOf(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID][]port.MemberView, error) {
	const op = "cabal.MembersOf"
	raw := make([]uuid.UUID, len(cabalIDs))
	for i, id := range cabalIDs {
		raw[i] = id.UUID()
	}
	rows, err := r.q.ListMembersOf(ctx, raw)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	members := make(map[ids.CabalID][]port.MemberView, len(cabalIDs))
	for _, id := range cabalIDs {
		members[id] = nil
	}
	for _, row := range rows {
		cabal, err := cabalID(op, row.CabalID)
		if err != nil {
			return nil, err
		}
		user, err := userID(op, row.UserID)
		if err != nil {
			return nil, err
		}
		members[cabal] = append(members[cabal], memberOf(user, row.Role, row.CanVote, row.JoinedAt))
	}
	return members, nil
}

func (r Postgres) VoterSet(ctx context.Context, id ids.CabalID) ([]ids.UserID, error) {
	const op = "cabal.VoterSet"
	rows, err := r.q.ListVoterIDs(ctx, id.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	voters := make([]ids.UserID, 0, len(rows))
	for _, raw := range rows {
		user, err := userID(op, raw)
		if err != nil {
			return nil, err
		}
		voters = append(voters, user)
	}
	return voters, nil
}

func (r Postgres) Rules(ctx context.Context, id ids.CabalID) (port.Rules, error) {
	row, err := r.find(ctx, "cabal.Rules", id)
	if err != nil {
		return port.Rules{}, err
	}
	return port.Rules{
		JoinMode:       domain.JoinMode(row.JoinMode),
		VoterMode:      domain.VoterMode(row.VoterMode),
		Threshold:      domain.Threshold(row.Threshold),
		ProposalExpiry: time.Duration(row.ProposalExpirySeconds) * time.Second,
		SlippageBps:    row.SlippageBps,
	}, nil
}

func (r Postgres) SlippageBps(ctx context.Context, id ids.CabalID) (int32, error) {
	row, err := r.find(ctx, "cabal.SlippageBps", id)
	if err != nil {
		return 0, err
	}
	return row.SlippageBps, nil
}

func (r Postgres) Status(ctx context.Context, id ids.CabalID) (port.Status, error) {
	row, err := r.find(ctx, "cabal.Status", id)
	if err != nil {
		return "", err
	}
	return port.Status(row.Status), nil
}

func (r Postgres) TreasuryWallet(ctx context.Context, id ids.CabalID) (port.TreasuryWallet, error) {
	const op = "cabal.TreasuryWallet"
	row, err := r.q.FindTreasuryWallet(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return port.TreasuryWallet{}, errs.New(errs.CodeCabalNotFound, op)
	case err != nil:
		return port.TreasuryWallet{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return walletOf(id, row.PrivyWalletID, row.Address), nil
}

func (r Postgres) TreasuryWallets(ctx context.Context) ([]port.TreasuryWallet, error) {
	const op = "cabal.TreasuryWallets"
	rows, err := r.q.ListTreasuryWallets(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	wallets := make([]port.TreasuryWallet, 0, len(rows))
	for _, row := range rows {
		id, err := cabalID(op, row.CabalID)
		if err != nil {
			return nil, err
		}
		wallets = append(wallets, walletOf(id, row.PrivyWalletID, row.Address))
	}
	return wallets, nil
}

func (r Postgres) CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error) {
	const op = "cabal.CabalsOf"
	rows, err := r.q.ListCabalIDsForUser(ctx, user.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	cabals := make([]ids.CabalID, 0, len(rows))
	for _, raw := range rows {
		id, err := cabalID(op, raw)
		if err != nil {
			return nil, err
		}
		cabals = append(cabals, id)
	}
	return cabals, nil
}

func (r Postgres) find(ctx context.Context, op string, id ids.CabalID) (sqlc.FindCabalRow, error) {
	row, err := r.q.FindCabal(ctx, id.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return sqlc.FindCabalRow{}, errs.New(errs.CodeCabalNotFound, op)
	case err != nil:
		return sqlc.FindCabalRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return row, nil
}

func viewOf(op string, row sqlc.FindCabalRow) (port.CabalView, error) {
	id, err := cabalID(op, row.ID)
	if err != nil {
		return port.CabalView{}, err
	}
	creator, err := userID(op, row.CreatorID)
	if err != nil {
		return port.CabalView{}, err
	}
	return port.CabalView{
		ID: id, Name: row.Name, PictureURL: row.PictureUrl.String, CreatorID: creator,
		Status: port.Status(row.Status), MemberCount: int(row.MemberCount), CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

func memberOf(user ids.UserID, role string, canVote bool, joinedAt time.Time) port.MemberView {
	return port.MemberView{UserID: user, Role: domain.Role(role), CanVote: canVote, JoinedAt: joinedAt.UTC()}
}

func walletOf(id ids.CabalID, privyWalletID, address string) port.TreasuryWallet {
	return port.TreasuryWallet{CabalID: id, PrivyWalletID: privyWalletID, Address: chain.SolanaAddress(address)}
}

func cabalID(op string, raw uuid.UUID) (ids.CabalID, error) {
	id, err := ids.ParseCabalID(raw.String())
	if err != nil {
		return ids.CabalID{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return id, nil
}

func userID(op string, raw uuid.UUID) (ids.UserID, error) {
	id, err := ids.ParseUserID(raw.String())
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return id, nil
}
