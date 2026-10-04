package treasury_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_servesActivityConsumesTradeEventsAndHasNoPollers(t *testing.T) {
	t.Parallel()
	m := treasury.New(module.Deps{})
	if m.Name() != "treasury" || !testkit.Serves(m.Mount, "GET", "/v1/me/txns") ||
		!testkit.Serves(m.Mount, "GET", "/v1/cabals/c/activity") || testkit.Serves(m.Mount, "GET", "/v1/cabals") ||
		m.Pollers() != nil {
		t.Fatalf("module = %s, pollers %v", m.Name(), m.Pollers())
	}
	var got []string
	for _, c := range m.Consumers() {
		for _, h := range c.Handlers {
			got = append(got, c.Durable+" "+h.Name+" "+string(h.Type()))
		}
	}
	want := []string{
		"treasury_trades treasury.trades " + string(events.TypeTradeConfirmed),
		"treasury_activity treasury.activity.submitted " + string(events.TypeTradeSubmitted),
		"treasury_activity treasury.activity.confirmed " + string(events.TypeTradeConfirmed),
		"treasury_activity treasury.activity.failed " + string(events.TypeTradeFailed),
		"treasury_user_ledger treasury.user_ledger " + string(events.TypeDepositCredited),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("consumers = %q, want %q", got, want)
	}
	if _, ok := m.Queries().(*adapters.Queries); !ok {
		t.Fatalf("Queries() = %T, want *adapters.Queries", m.Queries())
	}
	if m.SignatureOwner() == nil || m.WalletLedger() == nil {
		t.Fatal("signature owner or wallet ledger = nil")
	}
}

func TestModule_wireTakesMembersAndUsersFromTheBuiltSetAndFailsClosedWithout(t *testing.T) {
	t.Parallel()
	d := module.Deps{}
	alone := treasury.New(d)
	module.NewSet(alone)
	members, users := alone.Reads()
	if _, ok := members.(app.UnwiredReads); !ok {
		t.Fatalf("members = %T, want app.UnwiredReads", members)
	}
	if _, ok := users.(app.UnwiredReads); !ok {
		t.Fatalf("users = %T, want app.UnwiredReads", users)
	}
	wired := treasury.New(d)
	module.NewSet(wired, cabal.New(d), identity.New(d))
	members, users = wired.Reads()
	if reflect.TypeOf(members) != reflect.TypeOf(cabal.New(d).Queries()) ||
		reflect.TypeOf(users) != reflect.TypeOf(identity.New(d).Queries()) {
		t.Fatalf("reads = %T, %T, want cabal's and identity's Queries", members, users)
	}
}
