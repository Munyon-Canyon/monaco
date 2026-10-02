package referrals_test

import (
	"context"
	"io"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_registersTheReferralsDurableWithMintCodeOnUserCreated(t *testing.T) {
	t.Parallel()
	m := referrals.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	consumers := m.Consumers()
	if m.Name() != "referrals" || routes.ReferralsRoutes == nil || m.Pollers() != nil || len(consumers) != 1 ||
		consumers[0].Durable != "referrals" || len(consumers[0].Handlers) != 1 ||
		consumers[0].Handlers[0].Name != "referrals.mint_code" ||
		consumers[0].Handlers[0].Type() != events.TypeUserCreated {
		t.Fatalf("module = %s, routes %+v, consumers %+v, pollers %v", m.Name(), routes, consumers, m.Pollers())
	}
}

func TestModule_mintsFromCryptoRandUnlessGivenEntropy(t *testing.T) {
	t.Parallel()
	f := newMintFixture(t)
	scripted := referrals.WithEntropy(func(context.Context) io.Reader { return draws(0) })
	byDefault, fromScript := f.users.NewV7(), f.users.NewV7()
	for user, m := range map[uuid.UUID]*referrals.Module{
		byDefault:  referrals.New(module.Deps{}),
		fromScript: referrals.New(module.Deps{}, scripted),
	} {
		mint := m.Consumers()[0].Handlers[0]
		if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return mint.Apply(ctx, tx, events.UserCreated{V: 1, UserID: user}, deliveredAt())
		}); err != nil {
			t.Fatal(err)
		}
	}
	codes := f.codes(t)
	if !domain.IsRandomShape(codes[byDefault]) || codes[fromScript] != "22222222" {
		t.Fatalf("codes = %v, want a random code for %s and 22222222 for %s", codes, byDefault, fromScript)
	}
}
