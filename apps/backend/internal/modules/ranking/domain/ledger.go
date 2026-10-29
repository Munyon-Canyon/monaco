package domain

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	LedgerWindow         = 7 * 24 * time.Hour
	LedgerMinSharpest    = 2
	ledgerBpsPerUnit     = 10_000
	LedgerMarkerCall     = "call"
	LedgerMarkerBuy      = "buy"
	LedgerBuyMarkerLabel = "B"
)

func LedgerWeek(deliveryAt time.Time) (week string, labelStart, labelEnd, windowStart, windowEnd time.Time) {
	windowEnd = deliveryAt.UTC()
	windowStart = windowEnd.Add(-LedgerWindow)
	inside := windowStart.AddDate(0, 0, 1)
	labelStart = time.Date(inside.Year(), inside.Month(), inside.Day(), 0, 0, 0, 0, time.UTC)
	labelEnd = labelStart.AddDate(0, 0, 6)
	year, number := labelStart.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, number), labelStart, labelEnd, windowStart, windowEnd
}

type LedgerCabal struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	PictureURL *string   `json:"picture_url"`
}

type LedgerUser struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	PictureURL *string   `json:"picture_url"`
}

type LedgerSharpest struct {
	User     LedgerUser `json:"user"`
	RateBps  int        `json:"rate_bps"`
	Right    int        `json:"right"`
	Resolved int        `json:"resolved"`
}

type LedgerCallCabal struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type LedgerCall struct {
	ProposalID   uuid.UUID       `json:"proposal_id"`
	Cabal        LedgerCallCabal `json:"cabal"`
	Kind         string          `json:"kind"`
	Symbol       string          `json:"symbol"`
	AssetLogoURL *string         `json:"asset_logo_url"`
	YourChoice   string          `json:"your_choice"`
	MoveBps      int             `json:"move_bps"`
	Right        bool            `json:"right"`
	DecidedAt    time.Time       `json:"decided_at"`
}

type LedgerReply struct {
	CommentID     uuid.UUID  `json:"comment_id"`
	ProposalID    uuid.UUID  `json:"proposal_id"`
	ProposalLabel string     `json:"proposal_label"`
	Author        LedgerUser `json:"author"`
	Excerpt       string     `json:"excerpt"`
	ReactionCount int        `json:"reaction_count"`
	TopEmoji      string     `json:"top_emoji"`
	TopEmojiCount int        `json:"top_emoji_count"`
}

type LedgerPoint struct {
	At        time.Time `json:"at"`
	ReturnBps int64     `json:"return_bps"`
}

type LedgerMarker struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Symbol string    `json:"symbol"`
	Label  string    `json:"label"`
}

type Ledger struct {
	Week           string          `json:"week"`
	LabelStart     string          `json:"label_start"`
	LabelEnd       string          `json:"label_end"`
	WindowStart    time.Time       `json:"window_start"`
	WindowEnd      time.Time       `json:"window_end"`
	StakeReturnBps *int64          `json:"stake_return_bps"`
	Cabals         []LedgerCabal   `json:"cabals"`
	Voted          int             `json:"voted"`
	Eligible       int             `json:"eligible"`
	Right          int             `json:"right"`
	Resolved       int             `json:"resolved"`
	Sharpest       *LedgerSharpest `json:"sharpest"`
	BestCall       *LedgerCall     `json:"best_call"`
	Miss           *LedgerCall     `json:"miss"`
	Calls          []LedgerCall    `json:"calls"`
	TopReply       *LedgerReply    `json:"top_reply"`
	Series         []LedgerPoint   `json:"series"`
	Markers        []LedgerMarker  `json:"markers"`
}

func YourMove(kind, choice string, moveBps int) int {
	if (kind == CallBuy) == (choice == SideYes) {
		return moveBps
	}
	return -moveBps
}

func PickBest(calls []LedgerCall) *LedgerCall {
	return pickCall(calls, true, func(a, b LedgerCall) int { return cmp.Compare(a.MoveBps, b.MoveBps) })
}

func PickMiss(calls []LedgerCall) *LedgerCall {
	return pickCall(calls, false, func(a, b LedgerCall) int { return cmp.Compare(b.MoveBps, a.MoveBps) })
}

func pickCall(calls []LedgerCall, right bool, better func(a, b LedgerCall) int) *LedgerCall {
	var best *LedgerCall
	for i := range calls {
		c := calls[i]
		if c.Right != right {
			continue
		}
		if best == nil || cmp.Or(
			better(c, *best),
			c.DecidedAt.Compare(best.DecidedAt),
			strings.Compare(c.ProposalID.String(), best.ProposalID.String()),
		) > 0 {
			best = &calls[i]
		}
	}
	return best
}

func PickSharpest(records []LedgerSharpest) *LedgerSharpest {
	eligible := make([]LedgerSharpest, 0, len(records))
	for _, r := range records {
		if r.Resolved < LedgerMinSharpest {
			continue
		}
		r.RateBps = r.Right * ledgerBpsPerUnit / r.Resolved
		eligible = append(eligible, r)
	}
	if len(eligible) == 0 {
		return nil
	}
	slices.SortFunc(eligible, func(a, b LedgerSharpest) int {
		return cmp.Or(
			cmp.Compare(b.RateBps, a.RateBps),
			cmp.Compare(b.Right, a.Right),
			strings.Compare(a.User.Name, b.User.Name),
			strings.Compare(a.User.ID.String(), b.User.ID.String()),
		)
	})
	return &eligible[0]
}

func (l *Ledger) Tally() {
	for _, c := range l.Calls {
		l.Resolved++
		if c.Right {
			l.Right++
		}
	}
}
