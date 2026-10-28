package app

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
	Return1MBps    *int64
	TurnoutBps     *int64
	OpenProposal   *PreviewProposal
}

type PreviewProposal struct {
	ID           ids.ProposalID
	Kind         string
	Symbol       string
	AssetName    string
	AssetLogoURL string
	USDCMicros   *uint64
	ProposerName string
	Title        string
	Yes          int32
	No           int32
	Voted        int32
	Eligible     int32
	Replies      *int
	ExpiresAt    time.Time
}

type InvitePreview struct {
	Inviter *PreviewPerson
	Cabal   *PreviewCabal
}

type CabalPreviews interface {
	InvitePreview(ctx context.Context, cabal ids.CabalID, inviter ids.UserID) (cabalport.CabalPreview, bool, error)
}

type AssetsByMint interface {
	AssetByMint(ctx context.Context, mint market.Mint) (market.Asset, error)
}

type ReplyCounts interface {
	ReplyCounts(ctx context.Context, proposalIDs []ids.ProposalID) (map[ids.ProposalID]int, error)
}

type InvitePreviewDeps struct {
	Resolver Resolver
	Cabals   CabalPreviews
	Pots     treasuryport.PositionsReader
	Clock    clock.Clock
	Votes    governanceport.InviteFacts
	Returns  rankingport.CabalReturnsReader
	Assets   AssetsByMint
	Replies  ReplyCounts
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
	if err := q.fillMonth(ctx, preview.Cabal); err != nil {
		return InvitePreview{}, err
	}
	return preview, nil
}

func (q *InvitePreviewQuery) fillMonth(ctx context.Context, cabal *PreviewCabal) error {
	if q.d.Returns != nil {
		returns, err := q.d.Returns.CabalReturns(ctx, rankingport.Range1M, []ids.CabalID{cabal.ID})
		if err != nil {
			return err
		}
		cabal.Return1MBps = returns[cabal.ID].ReturnBps
	}
	if q.d.Votes == nil {
		return nil
	}
	now := q.d.Clock.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	voted, eligible, err := q.d.Votes.MonthTurnout(ctx, cabal.ID, from)
	if err != nil {
		return err
	}
	if eligible > 0 {
		bps := int64(voted) * 10000 / int64(eligible)
		cabal.TurnoutBps = &bps
	}
	open, ok, err := q.d.Votes.NewestOpenProposal(ctx, cabal.ID)
	if err != nil || !ok {
		return err
	}
	cabal.OpenProposal, err = q.proposal(ctx, open)
	return err
}

func (q *InvitePreviewQuery) proposal(ctx context.Context, open governanceport.OpenProposal) (*PreviewProposal, error) {
	mint, err := market.ParseMint(open.Mint)
	if err != nil {
		return nil, err
	}
	asset, err := q.d.Assets.AssetByMint(ctx, mint)
	if err != nil {
		return nil, err
	}
	cards, err := q.d.Resolver.Users.UsersByID(ctx, []ids.UserID{open.ProposerID})
	if err != nil {
		return nil, err
	}
	out := &PreviewProposal{
		ID: open.ID, Kind: open.Kind, Symbol: open.Symbol, AssetName: asset.DisplayName, AssetLogoURL: asset.LogoURL,
		USDCMicros: open.USDCMicros, ProposerName: cards[open.ProposerID].DisplayName, Title: firstLine(open.Thesis),
		Yes: open.Yes, No: open.No, Voted: open.Voted, Eligible: open.Eligible, ExpiresAt: open.ExpiresAt,
	}
	counts, err := q.d.Replies.ReplyCounts(ctx, []ids.ProposalID{open.ID})
	if err != nil {
		observability.Degraded(ctx, observability.ReferralsPreviewRepliesFailed,
			slog.String("proposal_id", open.ID.String()), slog.Any("error", err))
		return out, nil
	}
	replies := counts[open.ID]
	out.Replies = &replies
	return out, nil
}

func firstLine(thesis string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(thesis), "\n")
	return strings.TrimSpace(line)
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
