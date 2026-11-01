package notify_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const closeReminderBody = "You asked for a reminder. 4 yes, 1 no so far. Yours counts."

func (w voteWorld) closeReminder() app.CloseReminder {
	return app.CloseReminder{Cabals: w.ports.Cabals, Assets: w.ports.Assets, Clock: w.r.clock}
}

func (w voteWorld) closeReminderDue(
	proposal uuid.UUID, asker ids.UserID, kind, symbol string, expires time.Duration,
) events.ProposalCloseReminderDue {
	e := events.ProposalCloseReminderDue{
		V: 1, ProposalID: proposal, CabalID: w.cabalID.UUID(), UserID: asker.UUID(), Kind: kind, Symbol: symbol,
		ExpiresAt: w.r.clock.Now().Add(expires), Yes: 4, No: 1, Voters: 8,
	}
	if kind == "sell" {
		e.TokenAmount = 220_000_000
		return e
	}
	e.USDCMicros = money.MicrosFromUint64(400_000_000)
	return e
}

func (w voteWorld) deliverCloseReminder(t *testing.T, e events.ProposalCloseReminderDue) error {
	t.Helper()
	d := w.r.emit(t, opsActor, e)
	return handleKinds(t, w.r, w.r.sender, d, e, w.closeReminder())
}

func (w voteWorld) closePushTo(user ids.UserID, proposal uuid.UUID, title, body string) apns.Push {
	p := w.pushTo(user, proposal, title, body, voteCategory)
	p.Data["kind"] = "close_reminder"
	return p
}

func TestCloseReminder_rendersBuy(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)

	if err := w.deliverCloseReminder(
		t,
		w.closeReminderDue(proposal, w.members[5], "buy", "AAPLx", time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	sent := r.sender.Sent()
	want := w.closePushTo(w.members[5], proposal, "Buy AAPL $400 closes in 1h", closeReminderBody)
	if len(sent) != 1 || !equalPush(sent[0], want) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
}

func TestCloseReminder_rendersSellInMinutes(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)

	if err := w.deliverCloseReminder(
		t, w.closeReminderDue(proposal, w.members[5], "sell", "TSLAx", 40*time.Minute),
	); err != nil {
		t.Fatal(err)
	}

	sent := r.sender.Sent()
	if len(sent) != 1 || sent[0].Title != "Sell TSLA 2.2 sh closes in 40m" {
		t.Fatalf("sent %+v, want the sell title", sent)
	}
}

func TestCloseReminder_onlyTheAsker(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)

	if err := w.deliverCloseReminder(
		t,
		w.closeReminderDue(proposal, w.members[2], "buy", "AAPLx", time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	if got, want := w.sentTo(r), []ids.UserID{w.members[2]}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("pushed %v, want the asker alone %v", got, want)
	}
}

func TestCloseReminder_presentsAsAVotePush(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)

	if err := w.deliverCloseReminder(
		t,
		w.closeReminderDue(proposal, w.members[5], "buy", "AAPLx", time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	sent := r.sender.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent %+v, want one push", sent)
	}
	p := sent[0]
	if p.Category != voteCategory || p.InterruptionLevel != timeSensitive ||
		p.ThreadID != "cabal-"+w.cabalID.String() || p.Data["sender_kind"] != "cabal" ||
		p.Data["proposal_id"] != proposal.String() || p.Data["cabal_id"] != w.cabalID.String() ||
		p.Data["kind"] != "close_reminder" {
		t.Fatalf("push = %+v, want the VOTE category, time-sensitive, in the cabal thread, led by the cabal", p)
	}
}

func TestCloseReminder_sharesTheVoteReminderCollapseID(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.openAtOneThird(t)
	w.fixtureBallots(t, proposal)
	wantTick(t, w.reminders(), 1, 3)
	vote := r.sender.Sent()[0]

	if err := w.deliverCloseReminder(
		t,
		w.closeReminderDue(proposal, w.members[5], "buy", "AAPLx", time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	sent := r.sender.Sent()
	closing := sent[len(sent)-1]
	if closing.Data["kind"] != "close_reminder" || vote.Data["kind"] != "vote_reminder" {
		t.Fatalf("kinds = %q and %q, want the two reminders", vote.Data["kind"], closing.Data["kind"])
	}
	if closing.CollapseID != vote.CollapseID || closing.CollapseID != "proposal-"+proposal.String() {
		t.Fatalf("collapse ids = %q and %q, want both proposal-%s", vote.CollapseID, closing.CollapseID, proposal)
	}
}

func TestCloseReminder_decisionRequestsOffSendsNothing(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)
	r.exec(
		t,
		`INSERT INTO notification_preferences (user_id, category, enabled, updated_at) VALUES ($1, 'decision_requests', false, $2)`,
		w.members[5].UUID(),
		r.clock.Now(),
	)

	if err := w.deliverCloseReminder(
		t,
		w.closeReminderDue(proposal, w.members[5], "buy", "AAPLx", time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing with Decision requests off", sent)
	}
}

func TestCloseReminder_aRedeliveredEventSendsNothingTwice(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)
	e := w.closeReminderDue(proposal, w.members[5], "buy", "AAPLx", time.Hour)
	d := r.emit(t, opsActor, e)

	for range 2 {
		if err := handleKinds(t, r, r.sender, d, e, w.closeReminder()); err != nil {
			t.Fatal(err)
		}
	}

	if got := len(r.sender.Sent()); got != 1 {
		t.Fatalf("sends = %d after a redelivery, want 1", got)
	}
	r.wantCount(t, "close reminder rows", 1, `SELECT count(*) FROM notifications WHERE kind = 'close_reminder'`)
}

func TestCloseReminder_failsWhenItCannotRender(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.port")
	cabalID := closeID("1")
	for name, tc := range map[string]struct {
		kind, symbol string
		failAsset    bool
		failCabal    bool
		wantCode     errs.Code
	}{
		"an unknown action":      {kind: "hold", symbol: "AAPLx", wantCode: errs.CodeInvalidInput},
		"the asset read":         {kind: "buy", symbol: "AAPLx", failAsset: true, wantCode: errs.CodeDBUnavailable},
		"a sell of unknown size": {kind: "sell", symbol: "NOPEx", wantCode: errs.CodeAssetNotFound},
		"the cabal read":         {kind: "buy", symbol: "AAPLx", failCabal: true, wantCode: errs.CodeDBUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cabals := fakes.NewCabal(
				[]fakes.CabalSeed{{View: cabal.View{ID: ids.CabalIDFrom(cabalID), Name: cabalName}}},
				nil,
			)
			assets := marketfake.NewCatalog(marketfake.Fixtures()...)
			if tc.failAsset {
				assets.FailOnce("AssetBySymbol", down)
			}
			if tc.failCabal {
				cabals.Fail("Cabal", down)
			}
			kind := app.CloseReminder{Cabals: cabals, Assets: assets, Clock: testkit.NewClock(closeNow())}
			e := events.ProposalCloseReminderDue{
				V:          1,
				ProposalID: closeID("2"),
				CabalID:    cabalID,
				UserID:     closeID("3"),
				Kind:       tc.kind,
				Symbol:     tc.symbol,
				ExpiresAt:  closeNow().Add(time.Hour),
			}

			_, err := kind.Render(t.Context(), e, ids.UserIDFrom(e.UserID))

			if errs.CodeOf(err) != tc.wantCode {
				t.Fatalf("Render = %v, want code %v", err, tc.wantCode)
			}
		})
	}
}

func TestCloseReminder_namesAnUnknownBuyAssetGenerically(t *testing.T) {
	t.Parallel()
	cabalID := closeID("4")
	cabals := fakes.NewCabal([]fakes.CabalSeed{{View: cabal.View{ID: ids.CabalIDFrom(cabalID), Name: cabalName}}}, nil)
	kind := app.CloseReminder{
		Cabals: cabals, Assets: marketfake.NewCatalog(marketfake.Fixtures()...), Clock: testkit.NewClock(closeNow()),
	}
	e := events.ProposalCloseReminderDue{
		V: 1, ProposalID: closeID("5"), CabalID: cabalID, UserID: closeID("6"), Kind: "buy", Symbol: "NOPEx",
		USDCMicros: money.MicrosFromUint64(400_000_000), ExpiresAt: closeNow().Add(time.Hour),
	}

	msg, err := kind.Render(t.Context(), e, ids.UserIDFrom(e.UserID))

	if err != nil || msg.Title != "Buy a stock $400 closes in 1h" {
		t.Fatalf("Render = %q, %v, want the generic asset name", msg.Title, err)
	}
}

func closeNow() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }

func closeID(label string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("close-reminder:"+label))
}

func TestCloseReminder_nothingForAMemberWhoLeft(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8, 5)
	proposal := w.proposeNow(t)

	if err := w.deliverCloseReminder(
		t, w.closeReminderDue(proposal, w.members[5], "buy", "AAPLx", time.Hour),
	); err != nil {
		t.Fatal(err)
	}

	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing for a member who left the cabal", sent)
	}
}

func TestCloseReminder_recipientsFailWhenTheMemberListIsDown(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.members")
	r := newPushRig(t)
	e := events.ProposalCloseReminderDue{V: 1, ProposalID: r.ids.NewV7(), CabalID: r.ids.NewV7(), UserID: r.ids.NewV7()}

	got, err := app.CloseReminder{Cabals: membersDown{err: down}}.Recipients(t.Context(), e)

	if !errors.Is(err, down) || len(got) != 0 {
		t.Fatalf("Recipients = %v, %v, want no one and %v", got, err, down)
	}
}
