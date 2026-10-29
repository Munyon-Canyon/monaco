package app

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const getCabalOp = "cabal.GetCabal"

type UserCards interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]port.UserCard, error)
}

type Person struct {
	UserID      uuid.UUID
	Handle      string
	DisplayName string
	PhotoURL    string
}

type Member struct {
	Person
	Role     string
	CanVote  bool
	JoinedAt time.Time
}

type Membership struct {
	Role       string
	CanVote    bool
	CanPropose bool
}

type OpenVotes interface {
	OpenVoteCounts(ctx context.Context, cabals []ids.CabalID) (map[ids.CabalID]int, error)
}

type NoOpenVotes struct{}

func (NoOpenVotes) OpenVoteCounts(context.Context, []ids.CabalID) (map[ids.CabalID]int, error) {
	return map[ids.CabalID]int{}, nil
}

type Access struct {
	ID        uuid.UUID
	Direction string
	Status    string
}

type CabalView struct {
	ID              ids.CabalID
	Name            string
	PictureURL      *string
	Status          string
	JoinMode        string
	VoterMode       string
	Threshold       string
	ExpirySeconds   int32
	SlippageBps     int32
	ShowSides       bool
	ShowSidesSince  *time.Time
	MaxTradeBps     *int32
	CreatedAt       time.Time
	OpenProposals   int32
	VoterCount      int32
	Creator         Person
	Admins          []Person
	MemberCount     int32
	Members         []Member
	Me              *Membership
	Access          *Access
	TreasuryAddress chain.SolanaAddress
}

func GetCabal(
	ctx context.Context, q sqlc.DBTX, users UserCards, votes OpenVotes, cabalID ids.CabalID, actor ids.UserID,
) (CabalView, error) {
	dbq := sqlc.New(q)
	row, err := dbq.FindCabal(ctx, cabalID.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return CabalView{}, errs.New(errs.CodeCabalNotFound, getCabalOp)
	}
	if err != nil {
		return CabalView{}, errs.Wrap(err, errs.CodeInternal, getCabalOp)
	}
	memberRows, err := dbq.ListMembers(ctx, cabalID.UUID())
	if err != nil {
		return CabalView{}, errs.Wrap(err, errs.CodeInternal, getCabalOp)
	}
	wallet, err := dbq.FindTreasuryWallet(ctx, cabalID.UUID())
	if err != nil {
		return CabalView{}, errs.Wrap(err, errs.CodeInternal, getCabalOp)
	}
	cards, err := users.UsersByID(ctx, memberIDs(memberRows))
	if err != nil {
		return CabalView{}, errs.Wrap(err, errs.CodeInternal, getCabalOp)
	}
	open, err := votes.OpenVoteCounts(ctx, []ids.CabalID{cabalID})
	if err != nil {
		return CabalView{}, errs.Wrap(err, errs.CodeInternal, getCabalOp)
	}
	members, me := membersOf(memberRows, cards, actor.UUID())
	var access *Access
	if me == nil {
		access, err = requesterAccess(ctx, dbq, cabalID, actor)
		if err != nil {
			return CabalView{}, err
		}
	}
	view := CabalView{
		ID: cabalID, Name: row.Name, PictureURL: textPtr(row.PictureUrl.Valid, row.PictureUrl.String),
		Status: row.Status, JoinMode: row.JoinMode, VoterMode: row.VoterMode, Threshold: row.Threshold,
		ExpirySeconds: row.ProposalExpirySeconds, SlippageBps: row.SlippageBps,
		ShowSides: row.ShowSides, ShowSidesSince: timePtr(row.ShowSidesChangedAt.Valid, row.ShowSidesChangedAt.Time),
		MaxTradeBps: int32Ptr(row.MaxTradeBps.Valid, row.MaxTradeBps.Int32), CreatedAt: row.CreatedAt,
		OpenProposals: clampInt32(open[cabalID]), VoterCount: row.VoterCount,
		Creator: person(row.CreatorID, cards), Admins: adminsOf(memberRows, cards), MemberCount: row.MemberCount,
		Members: members, Me: me,
		Access: access, TreasuryAddress: chain.SolanaAddress(wallet.Address),
	}
	return view, nil
}

func requesterAccess(
	ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID, actor ids.UserID,
) (*Access, error) {
	row, err := q.FindRequesterAccess(ctx, sqlc.FindRequesterAccessParams{
		CabalID: cabalID.UUID(), UserID: actor.UUID(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		var none *Access
		return none, nil
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, getCabalOp)
	}
	return &Access{ID: row.ID, Direction: row.Direction, Status: row.Status}, nil
}

func memberIDs(rows []sqlc.ListMembersRow) []ids.UserID {
	out := make([]ids.UserID, 0, len(rows))
	for _, row := range rows {
		out = append(out, ids.UserIDFrom(row.UserID))
	}
	return out
}

func membersOf(
	rows []sqlc.ListMembersRow, cards map[ids.UserID]port.UserCard, actor uuid.UUID,
) ([]Member, *Membership) {
	members := make([]Member, 0, len(rows))
	var me *Membership
	for _, row := range rows {
		members = append(members, Member{
			Person: person(row.UserID, cards), Role: row.Role, CanVote: row.CanVote, JoinedAt: row.JoinedAt,
		})
		if row.UserID == actor {
			me = &Membership{Role: row.Role, CanVote: row.CanVote, CanPropose: row.CanVote}
		}
	}
	return members, me
}

func adminsOf(rows []sqlc.ListMembersRow, cards map[ids.UserID]port.UserCard) []Person {
	admins := make([]Person, 0, 2)
	for _, role := range []domain.Role{domain.RoleCreator, domain.RoleAdmin} {
		for _, row := range rows {
			if row.Role == string(role) {
				admins = append(admins, person(row.UserID, cards))
			}
		}
	}
	return admins
}

func person(id uuid.UUID, cards map[ids.UserID]port.UserCard) Person {
	card := cards[ids.UserIDFrom(id)]
	return Person{UserID: id, Handle: card.Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL}
}

func timePtr(ok bool, value time.Time) *time.Time {
	if !ok {
		return nil
	}
	return &value
}

func int32Ptr(ok bool, value int32) *int32 {
	if !ok {
		return nil
	}
	return &value
}

func clampInt32(n int) int32 {
	if n < 0 || n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(n)
}

func textPtr(ok bool, value string) *string {
	if !ok {
		return nil
	}
	return &value
}
