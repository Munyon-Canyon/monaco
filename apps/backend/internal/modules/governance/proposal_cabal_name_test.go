package governance_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type namedCabals struct {
	mu    sync.Mutex
	views map[ids.CabalID]cabalport.CabalView
	calls [][]ids.CabalID
	err   error
}

var _ cabalport.ViewReader = (*namedCabals)(nil)

func (n *namedCabals) set(id uuid.UUID, name string, status cabalport.Status) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.views == nil {
		n.views = map[ids.CabalID]cabalport.CabalView{}
	}
	cabalID := ids.CabalIDFrom(id)
	n.views[cabalID] = cabalport.CabalView{ID: cabalID, Name: name, Status: status}
}

func (n *namedCabals) Cabals(_ context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, slices.Clone(cabalIDs))
	if n.err != nil {
		return nil, n.err
	}
	found := make(map[ids.CabalID]cabalport.CabalView, len(cabalIDs))
	for _, id := range cabalIDs {
		if v, ok := n.views[id]; ok {
			found[id] = v
		}
	}
	return found, nil
}

func (n *namedCabals) Cabal(context.Context, ids.CabalID) (cabalport.CabalView, error) {
	return cabalport.CabalView{}, nil
}

func (n *namedCabals) Rules(context.Context, ids.CabalID) (cabalport.Rules, error) {
	return cabalport.Rules{}, nil
}

func (n *namedCabals) SlippageBps(context.Context, ids.CabalID) (int32, error) { return 0, nil }

func (n *namedCabals) Status(context.Context, ids.CabalID) (cabalport.Status, error) {
	return cabalport.StatusActive, nil
}

func (n *namedCabals) callCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.calls)
}

func (n *namedCabals) lastCall() []ids.CabalID {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.calls[len(n.calls)-1]
}

func (d readsDB) ctx(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
}

func (d readsDB) pendingIn(t *testing.T, cabal uuid.UUID, in time.Duration) {
	t.Helper()
	p := d.buy(d.caller)
	p.CabalID, p.ExpiresAt = cabal, d.now.Add(in)
	d.insert(t, p)
}

func TestHTTP_GetCabalProposals_CarriesCabalName(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	d.seed(t, d.now)
	d.seed(t, d.now.Add(time.Second))
	d.names.set(d.cabal, "Friday Night Lights", cabalport.StatusActive)
	h := adapters.HTTP{Reads: d.reads()}
	page := d.getProposals(t, h, api.GetCabalProposalsParams{})
	if len(page.Proposals) != 2 {
		t.Fatalf("proposals = %d, want 2", len(page.Proposals))
	}
	for _, p := range page.Proposals {
		if p.CabalName != "Friday Night Lights" {
			t.Errorf("cabal_name = %q, want Friday Night Lights", p.CabalName)
		}
	}
	if got := d.names.callCount(); got != 1 {
		t.Errorf("Cabals calls for a page of two rows = %d, want 1", got)
	}
	d.names.set(d.cabal, "Saturday Morning", cabalport.StatusActive)
	if got := d.getProposals(t, h, api.GetCabalProposalsParams{}).Proposals[0].CabalName; got != "Saturday Morning" {
		t.Errorf("cabal_name after a rename = %q, want Saturday Morning", got)
	}
}

func TestHTTP_GetProposal_CarriesCabalName(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.seed(t, d.now)
	d.names.set(d.cabal, "Friday Night Lights", cabalport.StatusActive)
	h := adapters.HTTP{Reads: d.reads()}
	get := func() api.ProposalDetail {
		res, err := h.GetProposal(d.ctx(t), api.GetProposalRequestObject{Id: id})
		got, ok := res.(api.GetProposal200JSONResponse)
		if err != nil || !ok {
			t.Fatalf("GetProposal = %#v, %v", res, err)
		}
		return api.ProposalDetail(got)
	}
	if got := get().CabalName; got != "Friday Night Lights" {
		t.Errorf("cabal_name = %q, want Friday Night Lights", got)
	}
	if got := d.names.callCount(); got != 1 {
		t.Errorf("Cabals calls for one proposal = %d, want 1", got)
	}
	d.names.set(d.cabal, "Saturday Morning", cabalport.StatusActive)
	if got := get().CabalName; got != "Saturday Morning" {
		t.Errorf("cabal_name after a rename = %q, want Saturday Morning", got)
	}
}

func TestHTTP_GetMyPendingVotes_CarriesCabalName(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	first, second := d.ids.NewV7(), d.ids.NewV7()
	d.names.set(first, "Friday Night Lights", cabalport.StatusActive)
	d.names.set(second, "Banned Club", cabalport.StatusBanned)
	d.pendingIn(t, first, time.Hour)
	d.pendingIn(t, second, 2*time.Hour)
	d.pendingIn(t, first, 3*time.Hour)
	h := adapters.HTTP{Reads: d.reads()}
	res, err := h.GetMyPendingVotes(d.ctx(t), api.GetMyPendingVotesRequestObject{})
	got, ok := res.(api.GetMyPendingVotes200JSONResponse)
	if err != nil || !ok || len(got) != 3 {
		t.Fatalf("GetMyPendingVotes = %#v, %v, want three rows", res, err)
	}
	for i, want := range []string{"Friday Night Lights", "Banned Club", "Friday Night Lights"} {
		if got[i].CabalName != want {
			t.Errorf("row %d cabal_name = %q, want %q", i, got[i].CabalName, want)
		}
	}
	if calls := d.names.callCount(); calls != 1 {
		t.Fatalf("Cabals calls for rows spanning two cabals = %d, want 1", calls)
	}
	if asked := d.names.lastCall(); len(asked) != 2 {
		t.Errorf("Cabals asked for %v, want the two distinct cabals", asked)
	}
}

func TestProposals_UnknownCabalNameIsEmpty(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	d.seed(t, d.now)
	page := d.list(t, app.FilterAll, 0, "")
	if len(page.Items) != 1 || page.Items[0].CabalName != "" {
		t.Errorf("items = %+v, want one with an empty cabal name", page.Items)
	}
}

func TestProposals_CabalReadFailureFailsTheRead(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	id := d.seed(t, d.now)
	d.pendingIn(t, d.cabal, time.Hour)
	d.names.err = errs.New(errs.CodeDBUnavailable, "t")
	reads, caller := d.reads(), ids.UserIDFrom(d.caller)
	_, listErr := reads.List(t.Context(), app.ListProposals{CabalID: ids.CabalIDFrom(d.cabal), Caller: caller})
	_, getErr := reads.Get(t.Context(), app.GetProposal{ID: ids.ProposalIDFrom(id), Caller: caller})
	_, pendingErr := reads.PendingVotes(t.Context(), caller)
	for name, err := range map[string]error{"List": listErr, "Get": getErr, "PendingVotes": pendingErr} {
		if errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Errorf("%s with the cabal read down = %v, want db_unavailable", name, err)
		}
	}
}

func TestHTTP_PostCabalProposal_CabalReadFailure(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	h.w.cabals.current.Load().Fail("Cabals", errs.New(errs.CodeDBUnavailable, "t"))
	member := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.w.members[0].String()})
	usdc := int64(5_000_000)
	adapter := adapters.HTTP{
		Propose: h.handler(h.d.ids),
		Reads:   app.NewProposalReads(h.d.pool, fakes.NewTrading(), h.w.cabals.current.Load()),
	}
	_, err := adapter.PostCabalProposal(member, api.PostCabalProposalRequestObject{
		Id: h.w.cabal.UUID(), Body: &api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc},
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("PostCabalProposal with the cabal read down = %v, want db_unavailable", err)
	}
}
