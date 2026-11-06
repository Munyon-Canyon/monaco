//go:build faultpoints

package social_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow22d_WriteMemberJoinedMoment_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	joined, err := domain.NewMemberJoinedMoment(f.member(0))
	if err != nil {
		t.Fatal(err)
	}
	cmd := app.WriteChatMoment{CabalID: f.cabal.ID, Moment: joined, At: f.clock.Now(), DedupeKey: "joined:crash"}
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		_, err := f.writer().Handle(ctx, cmd)
		return err
	})
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE kind = 'system'`); n != 1 {
		t.Fatalf("%d system rows after the restart, want 1", n)
	}
	if got := len(f.rt.Published()); got != 1 {
		t.Fatalf("%d moments published, want 1: nothing went out before the commit", got)
	}
}

func TestFlow22d_WriteProposalFailedMoment_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	settled, err := domain.NewProposalSettledMoment(domain.ProposalSettled{
		Outcome: domain.SettledFailed, ProposalID: f.deps.IDs.NewV7(), Side: domain.TradeBuy, Symbol: "NVDA",
		USDCMicros: 300_000_000, Yes: 2, No: 4, Voters: 6, ProposerID: f.member(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := app.WriteChatMoment{CabalID: f.cabal.ID, Moment: settled, At: f.clock.Now(), DedupeKey: "settled:crash"}
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		_, err := f.writer().Handle(ctx, cmd)
		return err
	})
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE kind = 'system'`); n != 1 {
		t.Fatalf("%d system rows after the restart, want 1", n)
	}
	if got := len(f.rt.Published()); got != 1 {
		t.Fatalf("%d moments published, want 1: nothing went out before the commit", got)
	}
}
