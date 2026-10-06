package app

import (
	"log/slog"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	allRange     = "ALL"
	cabalsBoard  = "cabals"
	peopleBoard  = "people"
	boardsOpName = "ranking.RunValuation.boards"
)

type boardInput struct {
	at       time.Time
	views    map[ids.CabalID]cabalport.CabalView
	members  map[ids.CabalID][]cabalport.MemberView
	stakes   []treasury.MemberStake
	users    map[ids.UserID]identity.UserCard
	valued   []CabalValue
	flagged  []CabalValue
	previous []Entry
}

type subject struct {
	id        uuid.UUID
	name      string
	handle    *string
	picture   *string
	createdAt time.Time
}

type person struct {
	user  identity.UserCard
	value money.Micros
	net   *big.Int
	flags []string
}

type boardBuilder struct {
	in      boardInput
	subject map[string]subject
	cabals  []domain.Candidate
	people  map[ids.UserID]*person
	entries []Entry
}

func buildEntries(in boardInput) ([]Entry, error) {
	b := &boardBuilder{in: in, subject: map[string]subject{}, people: map[ids.UserID]*person{}, entries: []Entry{}}
	byCabal := map[ids.CabalID][]treasury.MemberStake{}
	for _, stake := range in.stakes {
		byCabal[stake.CabalID] = append(byCabal[stake.CabalID], stake)
	}
	flagged := map[string][]domain.Flag{}
	for _, cabal := range in.flagged {
		flagged[cabal.CabalID.UUID().String()] = cabal.Flags
		flagged[MembersBoard(cabal.CabalID.UUID())] = cabal.Flags
		for _, stake := range byCabal[cabal.CabalID] {
			b.person(stake.UserID).flags = mergeFlags(b.person(stake.UserID).flags, cabal.Flags...)
		}
	}
	for _, cabal := range in.valued {
		if err := b.valued(cabal, byCabal[cabal.CabalID]); err != nil {
			return nil, err
		}
	}
	if err := b.carryPrevious(flagged); err != nil {
		return nil, err
	}
	if err := b.peopleBoard(); err != nil {
		return nil, err
	}
	if err := b.emit(cabalsBoard, domain.Rank(b.cabals, false)); err != nil {
		return nil, err
	}
	return b.entries, nil
}

func (b *boardBuilder) person(id ids.UserID) *person {
	if b.people[id] == nil {
		b.people[id] = &person{user: b.in.users[id], net: new(big.Int)}
	}
	return b.people[id]
}

type held struct {
	stake  treasury.MemberStake
	equity money.Micros
}

func (b *boardBuilder) valued(cabal CabalValue, stakes []treasury.MemberStake) error {
	view := b.in.views[cabal.CabalID]
	b.subject[view.ID.UUID().String()] = subject{
		id: view.ID.UUID(), name: view.Name, picture: optional(view.PictureURL), createdAt: view.CreatedAt,
	}
	holders, net := map[ids.UserID]held{}, new(big.Int)
	for _, stake := range stakes {
		equity, err := domain.MemberEquity(stake.ShareUnits, cabal.TotalShares, cabal.Value)
		if err != nil {
			return err
		}
		holders[stake.UserID] = held{stake: stake, equity: equity}
		net.Add(net, big.NewInt(stake.NetContributedMicros.Int64()))
		if err := b.addToPerson(stake, equity); err != nil {
			return err
		}
	}
	netMicros, err := signedOf(net)
	if err != nil {
		return err
	}
	candidate, err := lifetimeCandidate(view.ID.UUID().String(), view.CreatedAt, cabal.Value, netMicros)
	if err != nil {
		return err
	}
	if domain.Eligible(netMicros, money.Micros{}) {
		b.cabals = append(b.cabals, candidate)
	}
	return b.membersBoard(cabal.CabalID, holders)
}

func (b *boardBuilder) addToPerson(stake treasury.MemberStake, equity money.Micros) error {
	user, ok := b.in.users[stake.UserID]
	if !ok || user.Deleted {
		return nil
	}
	p := b.person(stake.UserID)
	p.net.Add(p.net, big.NewInt(stake.NetContributedMicros.Int64()))
	value, err := p.value.Add(equity)
	p.value = value
	return err
}

func (b *boardBuilder) membersBoard(cabalID ids.CabalID, holders map[ids.UserID]held) error {
	candidates := []domain.Candidate{}
	for _, member := range b.in.members[cabalID] {
		user, ok := b.in.users[member.UserID]
		if !ok || user.Deleted {
			continue
		}
		holder := holders[member.UserID]
		b.userSubject(user)
		candidate, err := lifetimeCandidate(
			user.ID.UUID().String(), user.CreatedAt, holder.equity, holder.stake.NetContributedMicros,
		)
		if err != nil {
			return err
		}
		candidates = append(candidates, candidate)
	}
	return b.emit(MembersBoard(cabalID.UUID()), domain.Rank(candidates, true))
}

func (b *boardBuilder) peopleBoard() error {
	candidates := []domain.Candidate{}
	for _, p := range b.people {
		net, err := signedOf(p.net)
		if err != nil {
			return err
		}
		if !domain.Eligible(net, money.Micros{}) {
			continue
		}
		b.userSubject(p.user)
		candidate, err := lifetimeCandidate(p.user.ID.UUID().String(), p.user.CreatedAt, p.value, net)
		if err != nil {
			return err
		}
		candidate.Flags = toFlags(p.flags)
		candidates = append(candidates, candidate)
	}
	return b.emit(peopleBoard, domain.Rank(candidates, false))
}

func (b *boardBuilder) carryPrevious(flagged map[string][]domain.Flag) error {
	for _, previous := range b.in.previous {
		if previous.Board != cabalsBoard {
			previous.Flags = mergeFlags(previous.Flags, flagged[previous.Board]...)
			b.entries = append(b.entries, previous)
			continue
		}
		value := previous.ValueMicros
		if value < 0 {
			return errs.New(errs.CodeDecodeFailed, boardsOpName)
		}
		b.subject[previous.SubjectID.String()] = subject{
			id: previous.SubjectID, name: previous.SubjectName, handle: previous.SubjectHandle,
			picture: previous.SubjectPictureURL, createdAt: previous.SubjectCreatedAt,
		}
		flags := mergeFlags(previous.Flags, flagged[previous.SubjectID.String()]...)
		b.cabals = append(b.cabals, previousCandidate(previous, uint64(value), flags))
	}
	return nil
}

func (b *boardBuilder) userSubject(user identity.UserCard) {
	b.subject[user.ID.UUID().String()] = subject{
		id: user.ID.UUID(), name: domain.SubjectName(user.DisplayName, user.Handle),
		handle: optional(user.Handle), picture: optional(user.PhotoURL), createdAt: user.CreatedAt,
	}
}

func (b *boardBuilder) emit(board string, rows []domain.Row) error {
	for _, row := range rows {
		value := row.Value.Uint64()
		if value > math.MaxInt64 {
			return errs.New(errs.CodeInvalidInput, boardsOpName)
		}
		subject := b.subject[row.SubjectID]
		var returnBps *int64
		if row.Return != nil {
			bps := int64(*row.Return)
			returnBps = &bps
		}
		b.entries = append(b.entries, Entry{
			Board: board, Range: allRange, Rank: row.Rank, SubjectID: subject.id, SubjectName: subject.name,
			SubjectHandle: subject.handle, SubjectPictureURL: subject.picture, SubjectCreatedAt: subject.createdAt,
			ValueMicros: int64(value), PnLMicros: row.PnL.Int64(), ReturnBps: returnBps,
			PricesAsOf: b.in.at, ComputedAt: b.in.at, Flags: fromFlags(row.Flags),
		})
	}
	return nil
}

func lifetimeCandidate(
	id string, createdAt time.Time, equity money.Micros, net money.SignedMicros,
) (domain.Candidate, error) {
	pnl, ret, err := domain.Lifetime(equity, net)
	return domain.Candidate{SubjectID: id, CreatedAt: createdAt, Value: equity, PnL: pnl, Return: ret}, err
}

func previousCandidate(previous Entry, value uint64, flags []string) domain.Candidate {
	candidate := domain.Candidate{
		SubjectID: previous.SubjectID.String(), CreatedAt: previous.SubjectCreatedAt,
		Value: money.MicrosFromUint64(value),
		PnL:   money.SignedMicrosFromInt64(previous.PnLMicros), Flags: toFlags(flags),
	}
	if previous.ReturnBps != nil {
		bps := domain.Bps(*previous.ReturnBps)
		candidate.Return = &bps
	}
	return candidate
}

func signedOf(n *big.Int) (money.SignedMicros, error) {
	if !n.IsInt64() {
		return money.SignedMicros{}, errs.New(errs.CodeInvalidInput, boardsOpName, slog.String("net", n.String()))
	}
	return money.SignedMicrosFromInt64(n.Int64()), nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mergeFlags(existing []string, added ...domain.Flag) []string {
	merged := append([]string{}, existing...)
	for _, flag := range added {
		if !slices.Contains(merged, string(flag)) {
			merged = append(merged, string(flag))
		}
	}
	return merged
}

func toFlags(flags []string) []domain.Flag {
	out := make([]domain.Flag, len(flags))
	for i, flag := range flags {
		out[i] = domain.Flag(flag)
	}
	return out
}

func fromFlags(flags []domain.Flag) []string {
	out := make([]string, len(flags))
	for i, flag := range flags {
		out[i] = string(flag)
	}
	return out
}
