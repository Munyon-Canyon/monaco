package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type member bool

func (m member) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) { return bool(m), nil }

type failingNames struct{}

func (failingNames) AssetNames(context.Context) (map[domain.Asset]app.AssetName, error) {
	return nil, errs.New(errs.CodeDBUnavailable, "test.AssetNames")
}

func TestActivityReads_aPageCostsAtMostFourQueries(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool, testkit.WithMembers(2))
	seedAAPLx(t, pool)
	mint := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	g, base := testkit.NewIDs(41), clock.Real{}.Now().UTC().Truncate(time.Second)
	for i := range 40 {
		seedActivity(t, pool, c.ID, seededActivity{
			id: g.NewV7(), at: base.Add(-time.Duration(i) * time.Second), status: "confirmed", asset: &mint,
			actor: &c.Members[i%2].ID,
		})
	}
	d := module.Deps{Pool: pool}
	reads := app.NewActivityReads(pool, cabal.New(d).Queries(), identity.New(d).Queries(),
		treasury.CatalogNames{Catalog: market.New(module.Deps{Pool: pool}).Catalog()})
	testkit.AssertQueries(t, "activity page", func() {
		page, err := reads.List(t.Context(), app.ListActivity{CabalID: c.ID, Caller: c.Creator.ID})
		if err != nil || len(page.Items) != 30 || page.Items[0].Actor == nil || page.Items[0].Asset == nil {
			t.Fatalf("page = %d items (first %+v), err %v; want 30 with actor and asset", len(page.Items),
				page.Items, err)
		}
	})
}

func TestActivityReads_refusesAPageOutOfRangeAndPassesPortErrorsOn(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabalID, user := ids.CabalIDFrom(testkit.NewIDs(51).NewV7()), ids.UserIDFrom(testkit.NewIDs(52).NewV7())
	mint, at := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", clock.Real{}.Now().UTC()
	seedActivity(t, pool, cabalID, seededActivity{
		id: testkit.NewIDs(53).NewV7(), at: at, status: "pending",
		asset: &mint, actor: &user,
	})
	names := treasury.CatalogNames{Catalog: market.New(module.Deps{Pool: pool}).Catalog()}
	tests := map[string]struct {
		reads *app.ActivityReads
		req   app.ListActivity
		code  errs.Code
	}{
		"limit over 100": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, names),
			app.ListActivity{Limit: 101},
			errs.CodeInvalidInput,
		},
		"members unwired": {
			app.NewActivityReads(pool, app.UnwiredReads{}, app.UnwiredReads{}, names),
			app.ListActivity{},
			errs.CodeUpstreamUnavailable,
		},
		"users unwired": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, names),
			app.ListActivity{},
			errs.CodeUpstreamUnavailable,
		},
		"names down": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, failingNames{}),
			app.ListActivity{},
			errs.CodeDBUnavailable,
		},
	}
	for name, tc := range tests {
		tc.req.CabalID, tc.req.Caller = cabalID, user
		if _, err := tc.reads.List(t.Context(), tc.req); errs.CodeOf(err) != tc.code {
			t.Errorf("%s: err = %v, want %s", name, err, tc.code)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := app.NewActivityReads(pool, member(true), app.UnwiredReads{}, names).List(ctx,
		app.ListActivity{CabalID: cabalID, Caller: user})
	if err == nil {
		t.Fatal("a canceled read succeeded")
	}
	if _, err := names.AssetNames(ctx); err == nil {
		t.Fatal("a canceled catalog read succeeded")
	}
}

func TestActivityHTTP_refusesACallerThatIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	tests := map[string]struct {
		actor *auth.Actor
		code  errs.Code
	}{
		"no actor": {nil, errs.CodeUnauthorized},
		"system":   {&auth.Actor{Kind: auth.ActorSystem, ID: "ops"}, errs.CodeForbidden},
		"bad user": {&auth.Actor{Kind: auth.ActorUser, ID: "nope"}, errs.CodeUnauthorized},
	}
	for name, tc := range tests {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		for _, call := range []func(context.Context) error{
			func(ctx context.Context) error {
				_, err := h.GetCabalActivity(ctx, api.GetCabalActivityRequestObject{})
				return err
			},
			func(ctx context.Context) error {
				_, err := h.GetMyTxns(ctx, api.GetMyTxnsRequestObject{})
				return err
			},
		} {
			if err := call(ctx); errs.CodeOf(err) != tc.code {
				t.Errorf("%s: err = %v, want %s", name, err, tc.code)
			}
		}
	}
}
