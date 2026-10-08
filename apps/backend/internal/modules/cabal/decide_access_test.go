package cabal_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) decide(
	ctx context.Context, user ids.UserID, c ids.CabalID, id uuid.UUID, decision app.Decision,
) (app.Access, error) {
	return app.NewDecideAccessHandler(f.uow, f.clock).Handle(as(ctx, user), app.DecideAccess{
		ActorID: user, CabalID: c, RequestID: ids.AccessRequestIDFrom(id), Decision: decision,
	})
}

func TestDecideAccess_approvingARequestAddsTheMemberInTheSameTransaction(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	got, err := f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Approve)
	if err != nil || got != (app.Access{ID: id, Direction: "request", Status: "approved"}) {
		t.Fatalf("DecideAccess = %+v, %v; want the request approved", got, err)
	}
	if m, ok := f.membership(t, c.ID, asker); !ok || m != (membership{"member", true}) {
		t.Fatalf("membership = %+v, %v; want a voting member", m, ok)
	}
	if row := f.accessRow(t, id); row.status != "approved" || *row.decidedBy != c.Creator.ID.String() {
		t.Fatalf("row = %+v, want approved by the creator", row)
	}
	decided := events.CabalAccessDecided{
		V: 1, RequestID: id, CabalID: c.ID.UUID(), UserID: asker.UUID(), Direction: "request",
		Decision: "approved", ActorID: c.Creator.ID.UUID(),
	}
	if got := decoded[events.CabalAccessDecided](t, f, events.TypeCabalAccessDecided); len(got) != 1 ||
		got[0] != decided {
		t.Fatalf("access_decided = %+v, want %+v", got, decided)
	}
	joined := events.CabalMemberJoined{
		V: 1, CabalID: c.ID.UUID(), UserID: asker.UUID(), Role: "member", Via: "request", RequestID: id,
	}
	if got := f.joins(t); len(got) != 1 || got[0] != joined {
		t.Fatalf("member_joined = %+v, want %+v", got, joined)
	}
}

func TestDecideAccess_reportsAnApprovalThatLostTheRaceToAConcurrentInsertAsAlreadyMember(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	commit := f.holdWrite(t, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, now())`, c.ID.UUID(), asker.UUID())
	done := make(chan error, 1)
	var deciding sync.WaitGroup
	t.Cleanup(deciding.Wait)
	deciding.Go(func() {
		_, err := f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Approve)
		done <- err
	})
	f.waitForALockWaiter(t)
	commit()
	wantErr(t, <-done, errs.CodeAlreadyMember)
	if row := f.accessRow(t, id); row.status != "pending" {
		t.Fatalf("request status = %s, want the losing approval rolled back", row.status)
	}
	if joins := f.joins(t); len(joins) != 0 {
		t.Fatalf("member_joined events = %+v, want none from the losing approval", joins)
	}
}

func TestDecideAccess_wrapsAMemberInsertFailureAsInternalAndKeepsTheRequestPending(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	id := f.filed(t, f.user(t), c.ID)
	f.exec(t, `ALTER TABLE cabal_members ADD CONSTRAINT no_members CHECK (role <> 'member')`)
	_, err := f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Approve)
	wantErr(t, err, errs.CodeInternal)
	if row := f.accessRow(t, id); row.status != "pending" {
		t.Fatalf("request status = %s, want the failed approval rolled back", row.status)
	}
}

func TestDecideAccess_approvingInAListVotingCabalAddsAMemberWhoCannotVote(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithVoterMode("list"))
	asker := f.user(t)
	if _, err := f.decide(t.Context(), c.Creator.ID, c.ID, f.filed(t, asker, c.ID), app.Approve); err != nil {
		t.Fatal(err)
	}
	if m, ok := f.membership(t, c.ID, asker); !ok || m.canVote {
		t.Fatalf("membership = %+v, %v; want a member without a vote", m, ok)
	}
}

func TestDecideAccess_denyingARequestAddsNoMember(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	got, err := f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Deny)
	if err != nil || got.Status != "denied" {
		t.Fatalf("DecideAccess = %+v, %v; want denied", got, err)
	}
	if _, ok := f.membership(t, c.ID, asker); ok || len(f.joins(t)) != 0 {
		t.Fatal("a denied request added a member")
	}
	if got := decoded[events.CabalAccessDecided](t, f, events.TypeCabalAccessDecided); len(got) != 1 ||
		got[0].Decision != "denied" {
		t.Fatalf("access_decided = %+v, want one denial", got)
	}
}

func TestDecideAccess_theInviteeAcceptsOrDeclinesAnInvite(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	accepter, decliner := f.user(t), f.user(t)
	accept := f.invite(t, c.ID, accepter, c.Creator.ID)
	decline := f.invite(t, c.ID, decliner, c.Creator.ID)
	if got, err := f.decide(t.Context(), accepter, c.ID, accept, app.Approve); err != nil || got.Status != "approved" {
		t.Fatalf("accept = %+v, %v", got, err)
	}
	if got, err := f.decide(t.Context(), decliner, c.ID, decline, app.Deny); err != nil || got.Status != "denied" {
		t.Fatalf("decline = %+v, %v", got, err)
	}
	joins := f.joins(t)
	if len(joins) != 1 || joins[0].UserID != accepter.UUID() || joins[0].Via != "invite" ||
		joins[0].RequestID != accept {
		t.Fatalf("member_joined = %+v, want the accepter via the invite", joins)
	}
	if _, ok := f.membership(t, c.ID, decliner); ok {
		t.Fatal("the decliner joined")
	}
}

func TestDecideAccess_refusesWithoutDecidingAnything(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	open := testkit.NewCabal(t, f.pool)
	asker, invitee, joiner, done := f.user(t), f.user(t), f.user(t), f.user(t)
	pending := f.filed(t, asker, c.ID)
	invite := f.invite(t, c.ID, invitee, c.Creator.ID)
	joined := f.invite(t, open.ID, joiner, open.Creator.ID)
	f.exec(t, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, now())`, open.ID.UUID(), joiner.UUID())
	settled := f.filed(t, done, c.ID)
	if _, err := f.decide(t.Context(), c.Creator.ID, c.ID, settled, app.Deny); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		user     ids.UserID
		cabal    ids.CabalID
		request  uuid.UUID
		decision app.Decision
		want     errs.Code
	}{
		{"an unknown decision", c.Creator.ID, c.ID, pending, "maybe", errs.CodeInvalidInput},
		{
			"an unknown cabal", c.Creator.ID, ids.CabalIDFrom(ids.Real{}.NewV7()), pending, app.Approve,
			errs.CodeCabalNotFound,
		},
		{"an unknown request", c.Creator.ID, c.ID, ids.Real{}.NewV7(), app.Approve, errs.CodeNotFound},
		{"a member on a request", c.Members[1].ID, c.ID, pending, app.Approve, errs.CodeNotCabalCreator},
		{"the asker on their own request", asker, c.ID, pending, app.Approve, errs.CodeNotCabalCreator},
		{"the creator on someone's invite", c.Creator.ID, c.ID, invite, app.Approve, errs.CodeForbidden},
		{"a request no longer pending", c.Creator.ID, c.ID, settled, app.Approve, errs.CodeAccessRequestNotPending},
		{"an invite for someone who already joined", joiner, open.ID, joined, app.Approve, errs.CodeAlreadyMember},
	} {
		if _, err := f.decide(t.Context(), tt.user, tt.cabal, tt.request, tt.decision); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if got := f.accessRow(t, joined).status; got != "pending" {
		t.Fatalf("invite status = %s, want the refused accept rolled back", got)
	}
	if got := decoded[events.CabalAccessDecided](t, f, events.TypeCabalAccessDecided); len(got) != 1 {
		t.Fatalf("access_decided events = %d, want only the setup denial", len(got))
	}
}

func TestDecideAccess_refusesToAdmitIntoABannedCabalOrOnAnExpiredInvite(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	invitee := f.user(t)
	invite := f.invite(t, c.ID, invitee, c.Creator.ID)
	f.clock.Advance(8 * 24 * time.Hour)
	_, err := f.decide(t.Context(), invitee, c.ID, invite, app.Approve)
	wantErr(t, err, errs.CodeInviteExpired)
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, c.ID.UUID())
	_, err = f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Approve)
	wantErr(t, err, errs.CodeCabalBanned)
	if got, err := f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Deny); err != nil || got.Status != "denied" {
		t.Fatalf("deny in a banned cabal = %+v, %v; want it allowed", got, err)
	}
}

func TestDecideAccess_twoConcurrentApprovalsAdmitTheMemberOnce(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	results := make(chan error, 2)
	start := make(chan struct{})
	var deciders sync.WaitGroup
	for range 2 {
		deciders.Go(func() {
			<-start
			_, err := f.decide(t.Context(), c.Creator.ID, c.ID, id, app.Approve)
			results <- err
		})
	}
	close(start)
	deciders.Wait()
	close(results)
	codes := map[errs.Code]int{}
	for err := range results {
		if err == nil {
			codes[""]++
			continue
		}
		codes[errs.CodeOf(err)]++
	}
	if codes[""] != 1 || codes[errs.CodeAccessRequestNotPending] != 1 {
		t.Fatalf("outcomes = %v, want one approval and one access_request_not_pending", codes)
	}
	if joins := f.joins(t); len(joins) != 1 {
		t.Fatalf("member_joined events = %d, want exactly one", len(joins))
	}
}

func TestPostCabalAccessDecision_wiresTheDecision(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	id := f.filed(t, f.user(t), c.ID)
	routes := f.routes(nil)
	req := func(decision api.AccessDecisionRequestDecision) api.PostCabalAccessDecisionRequestObject {
		return api.PostCabalAccessDecisionRequestObject{
			Id: c.ID.UUID(), RequestId: id, Body: &api.AccessDecisionRequest{Decision: decision},
		}
	}
	creator := as(t.Context(), c.Creator.ID)
	_, err := routes.PostCabalAccessDecision(t.Context(), req(api.Approve))
	wantErr(t, err, errs.CodeUnauthorized)
	_, err = routes.PostCabalAccessDecision(creator, api.PostCabalAccessDecisionRequestObject{Id: c.ID.UUID()})
	wantErr(t, err, errs.CodeInvalidInput)
	res, err := routes.PostCabalAccessDecision(creator, req(api.Approve))
	want := api.PostCabalAccessDecision200JSONResponse{Id: id, Direction: "request", Status: "approved"}
	if err != nil || res != want {
		t.Fatalf("PostCabalAccessDecision = %+v, %v; want %+v", res, err, want)
	}
	_, err = routes.PostCabalAccessDecision(creator, req(api.Deny))
	wantErr(t, err, errs.CodeAccessRequestNotPending)
}
