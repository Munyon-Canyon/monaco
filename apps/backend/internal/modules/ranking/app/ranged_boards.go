package app

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const unusableStart = "unusable_start"

type rangedSum struct {
	parts   []Ranged
	skipped bool
}

type memberResult struct {
	ranged  Ranged
	end     money.Micros
	skipped bool
}

func (b *boardBuilder) rangedCabal(
	cabal CabalValue,
	view cabalport.CabalView,
	holders map[ids.UserID]held,
	net money.SignedMicros,
) error {
	for i := range b.in.ranged {
		if err := b.rangedCabalIn(&b.in.ranged[i], cabal, view, holders, net); err != nil {
			return err
		}
	}
	return nil
}

func (b *boardBuilder) rangedCabalIn(
	book *rangedBook,
	cabal CabalValue,
	view cabalport.CabalView,
	holders map[ids.UserID]held,
	net money.SignedMicros,
) error {
	if err := b.rangedCabalRow(book, cabal, view, net); err != nil {
		return err
	}
	results, err := b.memberResults(book, cabal.CabalID, holders)
	if err != nil {
		return err
	}
	return b.rangedMembers(book, cabal.CabalID, results)
}

func (b *boardBuilder) rangedCabalRow(
	book *rangedBook,
	cabal CabalValue,
	view cabalport.CabalView,
	net money.SignedMicros,
) error {
	ranged, err := CabalRanged(
		book.Window, book.shares[cabal.CabalID], book.snaps[cabal.CabalID], cabal.Value, book.cabalFlows[cabal.CabalID],
	)
	if errors.Is(err, domain.ErrUnusableStart) {
		b.in.skipped(cabal.CabalID, book.Window, unusableStart)
		return b.carryFallback(view.ID.UUID(), string(book.Range))
	}
	if err != nil {
		return err
	}
	candidate, err := rangedCandidate(view.ID.UUID().String(), view.CreatedAt, cabal.Value, ranged)
	if err != nil {
		return err
	}
	if domain.Eligible(net, money.Micros{}) {
		rng := string(book.Range)
		b.cabals[rng] = append(b.cabals[rng], candidate)
	}
	return nil
}

func (b *boardBuilder) memberResults(
	book *rangedBook,
	cabalID ids.CabalID,
	holders map[ids.UserID]held,
) (map[ids.UserID]memberResult, error) {
	users := slices.Clone(book.users[cabalID])
	for userID := range holders {
		users = append(users, userID)
	}
	for _, member := range b.in.members[cabalID] {
		users = append(users, member.UserID)
	}
	results := map[ids.UserID]memberResult{}
	skipped := false
	for _, userID := range users {
		if _, done := results[userID]; done {
			continue
		}
		key := MemberKey{UserID: userID, CabalID: cabalID}
		ranged, err := b.in.memberRangedOrDefault()(
			book.Window, book.shares[cabalID], book.stakes[key], book.snaps[cabalID], holders[userID].equity,
			book.flows[key],
		)
		skip := errors.Is(err, domain.ErrUnusableStart)
		if err != nil && !skip {
			return nil, err
		}
		results[userID] = memberResult{ranged: ranged, end: holders[userID].equity, skipped: skip}
		skipped = skipped || skip
		b.addToRangedPerson(book, userID, results[userID])
	}
	if skipped {
		b.in.skipped(cabalID, book.Window, unusableStart)
	}
	return results, nil
}

func (b *boardBuilder) addToRangedPerson(book *rangedBook, userID ids.UserID, result memberResult) {
	user, ok := b.in.users[userID]
	if !ok || user.Deleted {
		return
	}
	p := b.person(userID)
	rng := string(book.Range)
	if p.sums[rng] == nil {
		p.sums[rng] = &rangedSum{}
	}
	if result.skipped {
		p.sums[rng].skipped = true
		return
	}
	p.sums[rng].parts = append(p.sums[rng].parts, result.ranged)
}

func (b *boardBuilder) rangedMembers(book *rangedBook, cabalID ids.CabalID, results map[ids.UserID]memberResult) error {
	candidates := []domain.Candidate{}
	for _, member := range b.in.members[cabalID] {
		user, ok := b.in.users[member.UserID]
		if !ok || user.Deleted {
			continue
		}
		b.userSubject(user)
		result := results[member.UserID]
		candidate := domain.Candidate{
			SubjectID: user.ID.UUID().String(), CreatedAt: user.CreatedAt, Value: result.end,
		}
		if result.skipped {
			candidate.Flags = []domain.Flag{domain.FlagStalePrices}
		} else {
			var err error
			if candidate, err = rangedCandidate(
				user.ID.UUID().String(), user.CreatedAt, result.end, result.ranged,
			); err != nil {
				return err
			}
		}
		candidates = append(candidates, candidate)
	}
	return b.emit(MembersBoard(cabalID.UUID()), string(book.Range), domain.Rank(candidates, true))
}

func (b *boardBuilder) rangedPerson(p *person, all domain.Candidate, out map[string][]domain.Candidate) error {
	for i := range b.in.ranged {
		book := &b.in.ranged[i]
		rng := string(book.Range)
		sum := p.sums[rng]
		if sum == nil {
			continue
		}
		if len(sum.parts) == 0 {
			out[rng] = append(out[rng], domain.Candidate{
				SubjectID: all.SubjectID, CreatedAt: all.CreatedAt, Value: all.Value,
				Flags: toFlags(mergeFlags(p.flags, domain.FlagStalePrices)),
			})
			continue
		}
		total, err := SumRanged(book.T0, book.T1, sum.parts)
		if err != nil {
			return err
		}
		candidate, err := rangedCandidate(all.SubjectID, all.CreatedAt, all.Value, total)
		if err != nil {
			return err
		}
		candidate.Flags = toFlags(p.flags)
		if sum.skipped {
			candidate.Flags = toFlags(mergeFlags(p.flags, domain.FlagStalePrices))
		}
		if candidate.Return != nil || sum.skipped {
			out[rng] = append(out[rng], candidate)
		}
	}
	return nil
}

func (b *boardBuilder) carryFallback(cabalID uuid.UUID, rng string) error {
	previous, ok := b.in.fallback[fallbackKey{cabal: cabalID, rng: rng}]
	if !ok {
		return nil
	}
	return b.carryCabalRow(previous, mergeFlags(previous.Flags, domain.FlagStalePrices))
}

func rangedCandidate(
	id string, createdAt time.Time, value money.Micros, ranged Ranged,
) (domain.Candidate, error) {
	gain, ret, err := ranged.Gain()
	return domain.Candidate{SubjectID: id, CreatedAt: createdAt, Value: value, PnL: gain, Return: ret}, err
}
