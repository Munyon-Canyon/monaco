package social_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
)

func (w scopeWorld) detailVisible(t *testing.T, id uuid.UUID, scope string) bool {
	t.Helper()
	res, err := w.f.routes().GetFeedItem(asUser(t.Context(), w.viewer), api.GetFeedItemRequestObject{
		Id: id, Params: api.GetFeedItemParams{Scope: ptr(api.GetFeedItemParamsScope(scope))},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := res.(api.GetFeedItem200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return got.Visible
}

func TestGetFeed_ScopeNetworkAndOthersOverHTTP(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	for scope, want := range map[string][]uuid.UUID{
		"network": {w.bByFriend, w.aByFriend, w.aByStranger},
		"others":  {w.cByStranger, w.bByStranger, w.bByFriend},
	} {
		page := w.f.getFeed(t, w.viewer, api.GetFeedParams{Scope: ptr(api.GetFeedParamsScope(scope))})
		if got := feedIDs(page); !sameSet(got, want) {
			t.Errorf("scope=%s lists %v, want %v", scope, got, want)
		}
	}
}

func TestGetFeedItem_ScopeNetworkAndOthersOverHTTP(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	for _, tt := range []struct {
		name  string
		id    uuid.UUID
		scope string
		want  bool
	}{
		{"a cabal I am not in, by a stranger, in network", w.cByStranger, "network", false},
		{"a cabal I am not in, by a friend, in network", w.bByFriend, "network", true},
		{"my cabal, by a stranger, in network", w.aByStranger, "network", true},
		{"my cabal in others", w.aByStranger, "others", false},
		{"a cabal I am not in, in others", w.cByStranger, "others", true},
		{"a price move in network", w.move, "network", false},
		{"a price move in others", w.move, "others", false},
	} {
		if got := w.detailVisible(t, tt.id, tt.scope); got != tt.want {
			t.Errorf("%s: visible = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGetFeed_StatusOpenPagesToTheEndPastNewerClosedProposals(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	post := func(status string) uuid.UUID {
		w.f.clock.Advance(time.Second)
		return w.f.item(t, func(it *feed.Item) {
			it.Kind, it.CabalID, it.ActorID, it.Status = feed.KindProposal, w.cabalA, w.stranger, status
		})
	}
	oldest := post("open")
	for range 6 {
		post("passed")
	}
	newest := post("open")
	params := api.GetFeedParams{
		Kind: ptr("proposal"), Scope: ptr(api.GetFeedParamsScope("mine")), Status: ptr(api.GetFeedParamsStatus("open")),
		Limit: ptr(1),
	}
	first := w.f.getFeed(t, w.viewer, params)
	if got := feedIDs(first); !slices.Equal(got, []uuid.UUID{newest}) || first.NextCursor == nil {
		t.Fatalf("first page = %v, cursor %v, want %s and a cursor", got, first.NextCursor, newest)
	}
	params.Cursor = first.NextCursor
	second := w.f.getFeed(t, w.viewer, params)
	if got := feedIDs(second); !slices.Equal(got, []uuid.UUID{oldest}) || second.NextCursor != nil {
		t.Fatalf("second page = %v, cursor %v, want %s and no cursor", got, second.NextCursor, oldest)
	}
}

func TestGetFeed_StatusClosedIsInvalidInput(t *testing.T) {
	t.Parallel()
	w := newScopeWorld(t)
	_, err := w.f.routes().GetFeed(asUser(t.Context(), w.viewer), api.GetFeedRequestObject{
		Params: api.GetFeedParams{Status: ptr(api.GetFeedParamsStatus("closed"))},
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("status=closed err = %v, want invalid_input", err)
	}
}
