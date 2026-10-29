package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestUpdateCabal_refusesEveryRuleFieldWithRulesChangeByProposal(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	before := f.stored(t, c.ID)
	for _, tt := range []struct {
		name string
		body api.UpdateCabalRequest
	}{
		{"join_mode", api.UpdateCabalRequest{JoinMode: ptr("request")}},
		{"voter_mode", api.UpdateCabalRequest{VoterMode: ptr("list")}},
		{"voter_ids", api.UpdateCabalRequest{VoterIds: voterIDs(c.Creator)}},
		{"threshold", api.UpdateCabalRequest{Threshold: ptr("unanimous")}},
		{"proposal_expiry_seconds", api.UpdateCabalRequest{ProposalExpirySeconds: ptr(int32(3600))}},
	} {
		_, err := f.patch(t.Context(), c.Creator.ID, c.ID, tt.body)
		if errs.CodeOf(err) != errs.CodeRulesChangeByProposal {
			t.Errorf("%s: err = %v, want rules_change_by_proposal", tt.name, err)
		}
	}
	if after := f.stored(t, c.ID); after != before || len(f.updates(t)) != 0 {
		t.Fatalf("stored %+v (was %+v), events %d; want nothing written", after, before, len(f.updates(t)))
	}
}

func TestUpdateCabal_stillRenamesAndSetsSlippage(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	f.patchOK(t, c, api.UpdateCabalRequest{Name: ptr("Renamed pot"), SlippageBps: ptr(int32(40))})
	if got := f.stored(t, c.ID); got.name != "Renamed pot" || got.slippage != 40 {
		t.Fatalf("stored %+v, want the new name and slippage", got)
	}
}
