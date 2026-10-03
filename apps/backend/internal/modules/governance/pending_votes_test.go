package governance_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (d readsDB) expiringFor(t *testing.T, in time.Duration, voters ...uuid.UUID) uuid.UUID {
	t.Helper()
	p := d.buy(voters...)
	p.CabalID, p.ExpiresAt = d.ids.NewV7(), d.now.Add(in)
	d.insert(t, p)
	return p.ID
}

func (d readsDB) pending(t *testing.T, voter uuid.UUID) []uuid.UUID {
	t.Helper()
	got, err := d.reads().PendingVotes(t.Context(), ids.UserIDFrom(voter))
	if err != nil {
		t.Fatalf("PendingVotes = %v", err)
	}
	out := make([]uuid.UUID, len(got))
	for i, v := range got {
		out[i] = v.ProposalID.UUID()
	}
	return out
}

func TestMe_PendingVotes_ExcludesVoted(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	other := d.ids.NewV7()
	later := d.expiringFor(t, 3*time.Hour, d.caller, other)
	sooner := d.expiringFor(t, time.Hour, d.caller)
	voted := d.expiringFor(t, 2*time.Hour, d.caller, other)
	d.expiringFor(t, time.Hour, other)
	closed := d.expiringFor(t, time.Hour, d.caller)
	d.setStatus(t, closed, string(domain.StatusExpired), "")
	d.ballot(t, later, other, "yes")
	if got, want := d.pending(t, d.caller), []uuid.UUID{sooner, voted, later}; !slices.Equal(got, want) {
		t.Fatalf("pending before the ballot = %v, want %v soonest first", got, want)
	}
	d.ballot(t, voted, d.caller, "no")
	if got, want := d.pending(t, d.caller), []uuid.UUID{sooner, later}; !slices.Equal(got, want) {
		t.Errorf("pending after the ballot = %v, want %v", got, want)
	}
}

func TestMe_PendingVotes_capsAtFifty(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	for i := range 51 {
		d.expiringFor(t, time.Duration(i+1)*time.Minute, d.caller)
	}
	if got := d.pending(t, d.caller); len(got) != 50 {
		t.Errorf("pending = %d proposals, want 50", len(got))
	}
}

func TestMe_PendingVotes_failures(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.expiringFor(t, time.Hour, d.caller)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.reads().PendingVotes(ctx, ids.UserIDFrom(d.caller)); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("PendingVotes on a cancelled context err = %v, want internal", err)
	}
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposals SET kind = 'swap' WHERE id = $1`, p); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reads().PendingVotes(t.Context(), ids.UserIDFrom(d.caller)); errs.CodeOf(
		err) != errs.CodeDecodeFailed {
		t.Errorf("PendingVotes with an unknown stored kind err = %v, want decode_failed", err)
	}
}

func TestHTTP_GetMyPendingVotes(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.buy(d.caller)
	d.insert(t, p)
	h := adapters.HTTP{Reads: d.reads()}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetMyPendingVotes(ctx, api.GetMyPendingVotesRequestObject{})
	got, ok := res.(api.GetMyPendingVotes200JSONResponse)
	if err != nil || !ok || len(got) != 1 {
		t.Fatalf("GetMyPendingVotes = %#v, %v, want one proposal", res, err)
	}
	want := api.PendingVote{
		ProposalId: p.ID, CabalId: p.CabalID, Kind: "buy", Symbol: "AAPLx", ExpiresAt: got[0].ExpiresAt,
	}
	if got[0] != want || !got[0].ExpiresAt.Equal(p.ExpiresAt) {
		t.Errorf("pending vote = %+v, want %+v expiring at %s", got[0], want, p.ExpiresAt)
	}
	if _, err := h.GetMyPendingVotes(t.Context(), api.GetMyPendingVotesRequestObject{}); errs.CodeOf(
		err) != errs.CodeUnauthorized {
		t.Errorf("GetMyPendingVotes with no actor err = %v, want unauthorized", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := h.GetMyPendingVotes(cancelled, api.GetMyPendingVotesRequestObject{}); errs.CodeOf(
		err) != errs.CodeInternal {
		t.Errorf("GetMyPendingVotes on a cancelled context err = %v, want internal", err)
	}
}
