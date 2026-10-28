package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type PreviewPerson struct {
	DisplayName string
	PhotoURL    string
}

type PreviewCabal struct {
	ID             ids.CabalID
	Name           string
	PictureURL     string
	MemberCount    int32
	Since          time.Time
	JoinMode       string
	FirstMembers   []PreviewPerson
	PotValueMicros money.Micros
}

type InvitePreview struct {
	Inviter *PreviewPerson
	Cabal   *PreviewCabal
}

type CabalPreviews interface {
	InvitePreview(ctx context.Context, cabal ids.CabalID, inviter ids.UserID) (cabalport.CabalPreview, bool, error)
}

type InvitePreviewDeps struct {
	Resolver Resolver
	Cabals   CabalPreviews
	Pots     treasuryport.PositionsReader
}

type InvitePreviewQuery struct{ d InvitePreviewDeps }

func NewInvitePreviewQuery(d InvitePreviewDeps) *InvitePreviewQuery { return &InvitePreviewQuery{d: d} }

func (q *InvitePreviewQuery) Run(ctx context.Context, code string, cabal ids.CabalID) (InvitePreview, error) {
	resolved, err := q.d.Resolver.Resolve(ctx, code)
	if errs.CodeOf(err) == errs.CodeReferralCodeUnknown {
		return InvitePreview{}, nil
	}
	if err != nil {
		return InvitePreview{}, err
	}
	card, err := q.d.Resolver.Referrer(ctx, resolved)
	if err != nil {
		return InvitePreview{}, err
	}
	inviter := personOf(card)
	preview := InvitePreview{Inviter: &inviter}
	found, ok, err := q.d.Cabals.InvitePreview(ctx, cabal, resolved.UserID)
	if err != nil {
		return InvitePreview{}, err
	}
	if !ok || found.Banned || !found.InviterIsMember {
		return preview, nil
	}
	first, err := q.people(ctx, found.FirstMemberIDs)
	if err != nil {
		return InvitePreview{}, err
	}
	pot, err := q.d.Pots.PotValue(ctx, cabal)
	if err != nil {
		return InvitePreview{}, err
	}
	preview.Cabal = &PreviewCabal{
		ID: found.ID, Name: found.Name, PictureURL: found.PictureURL, MemberCount: found.MemberCount,
		Since: found.CreatedAt, JoinMode: string(found.JoinMode), FirstMembers: first, PotValueMicros: pot,
	}
	return preview, nil
}

func (q *InvitePreviewQuery) people(ctx context.Context, userIDs []ids.UserID) ([]PreviewPerson, error) {
	cards, err := q.d.Resolver.Users.UsersByID(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	people := make([]PreviewPerson, 0, len(userIDs))
	for _, id := range userIDs {
		card, ok := cards[id]
		if !ok || card.Deleted {
			continue
		}
		people = append(people, personOf(card))
	}
	return people, nil
}

func personOf(card identity.UserCard) PreviewPerson {
	return PreviewPerson{DisplayName: card.DisplayName, PhotoURL: card.PhotoURL}
}
