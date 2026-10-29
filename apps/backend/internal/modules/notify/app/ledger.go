package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	weeklyLedgerKind     = "weekly_ledger"
	weeklyLedgerThread   = "ledger"
	weeklyLedgerCategory = "LEDGER"
	minusSign            = "−"
	bpsPerTenth          = 10
	tenthsPerPercent     = 10
	opWeeklyLedger       = "notify.WeeklyLedger"
)

type WeeklyLedger struct {
	Users   Users
	Ledgers rankingport.Ledgers
}

func (WeeklyLedger) Name() string { return "weekly_ledger" }

func (k WeeklyLedger) Recipients(ctx context.Context, e events.RankingLedgerReady) ([]ids.UserID, error) {
	user := ids.UserIDFrom(e.UserID)
	cards, err := k.Users.UsersByID(ctx, []ids.UserID{user})
	if err != nil {
		return nil, err
	}
	card := cards[user]
	if card.Deleted || card.AccountStatus != identity.AccountActive {
		return nil, nil
	}
	_, found, err := k.Ledgers.Ledger(ctx, user, e.Week)
	if err != nil || !found {
		return nil, err
	}
	return []ids.UserID{user}, nil
}

func (k WeeklyLedger) Render(ctx context.Context, e events.RankingLedgerReady, to ids.UserID) (Message, error) {
	ledger, found, err := k.Ledgers.Ledger(ctx, to, e.Week)
	if err != nil {
		return Message{}, err
	}
	if !found {
		return Message{}, errs.New(errs.CodeInternal, opWeeklyLedger, slog.String("week", e.Week))
	}
	presentation := ForAccount()
	presentation.ThreadID, presentation.Category = weeklyLedgerThread, weeklyLedgerCategory
	presentation.Sender = Sender{Kind: SenderApp}
	return Message{
		Title:        ledgerTitle(e.Week),
		Body:         ledgerBody(ledger),
		Data:         map[string]string{"kind": weeklyLedgerKind, "week": e.Week},
		CollapseID:   "ledger-" + e.Week,
		Presentation: presentation,
	}, nil
}

func ledgerTitle(week string) string {
	_, number, _ := strings.Cut(week, "-W")
	n, _ := strconv.Atoi(number)
	return fmt.Sprintf("The Ledger · Week %d", n)
}

func ledgerBody(l rankingport.Ledger) string {
	voted := fmt.Sprintf("You voted %d of %d.", l.Voted, l.Eligible)
	switch {
	case l.BestCall != nil:
		return fmt.Sprintf("%s Best call: %s %s, +%s%%.", voted, callSide(*l.BestCall),
			strings.TrimSuffix(l.BestCall.Symbol, "x"), tenths(int64(l.BestCall.MoveBps)))
	case l.Resolved > 0:
		return voted + " No call landed this week."
	case l.StakeReturnBps == nil:
		return voted
	}
	bps := *l.StakeReturnBps
	sign := "+"
	if bps < 0 {
		sign, bps = minusSign, -bps
	}
	return fmt.Sprintf("Your stake %s%s%% this week.", sign, tenths(bps))
}

func callSide(c rankingport.LedgerCall) string {
	if (c.Kind == rankingport.CallBuy) == (c.YourChoice == rankingport.SideYes) {
		return "Buy"
	}
	return "Sell"
}

func tenths(bps int64) string {
	t := (bps + bpsPerTenth/2) / bpsPerTenth
	return fmt.Sprintf("%d.%d", t/tenthsPerPercent, t%tenthsPerPercent)
}
