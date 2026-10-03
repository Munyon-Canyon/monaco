package events

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type widgetBumped struct {
	V  int       `json:"v"`
	ID uuid.UUID `json:"id"`
	N  int       `json:"n"`
}

func (widgetBumped) Type() Type { return "widget.bumped" }

func (widgetBumped) AggregateType() string { return "widget" }

func (e widgetBumped) AggregateID() uuid.UUID { return e.ID }

func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		if got := recover(); got != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()
	fn()
}

func TestSubjects(t *testing.T) {
	t.Parallel()
	want := []string{
		"events.asset.price_moved",
		"events.cabal.access_decided", "events.cabal.access_requested", "events.cabal.created",
		"events.cabal.member_joined", "events.cabal.member_left", "events.cabal.updated",
		"events.deposit.credited",
		"events.follow.created", "events.follow.removed",
		"events.proposal.created", "events.proposal.executed", "events.proposal.execution_blocked",
		"events.proposal.expired", "events.proposal.failed", "events.proposal.passed", "events.proposal.voided",
		"events.proposal.withdrawn", "events.system.pinged", "events.trade.blocked", "events.trade.confirmed",
		"events.trade.failed", "events.trade.submitted", "events.user.auth_state_changed", "events.user.created",
		"events.user.deleted", "events.user.nudge_due", "events.user.profile_updated",
	}
	if got := Subjects(); !slices.Equal(got, want) {
		t.Fatalf("Subjects() = %q, want %q", got, want)
	}
}

func TestRegistrationsKeepTheSingleListOrder(t *testing.T) {
	t.Parallel()
	single := []string{
		"system.pinged v1",
		"trade.blocked v1", "trade.submitted v1", "trade.confirmed v1", "trade.failed v1",
		"proposal.created v1", "proposal.passed v1", "proposal.failed v1", "proposal.expired v1",
		"proposal.withdrawn v1", "proposal.voided v1", "proposal.executed v1", "proposal.execution_blocked v1",
		"cabal.created v1", "cabal.member_joined v1", "cabal.access_requested v1", "cabal.access_decided v1",
		"cabal.member_left v1", "cabal.updated v1",
		"price.tick v1 core",
		"user.created v1", "user.auth_state_changed v1", "user.profile_updated v1", "user.deleted v1",
		"user.nudge_due v1",
	}
	regs := registrations()
	got := make([]string, 0, len(regs))
	for _, r := range regs {
		entry := fmt.Sprintf("%s v%d", r.typ, r.current)
		if r.core {
			entry += " core"
		}
		got = append(got, entry)
	}
	rest := got
	for _, w := range single {
		i := slices.Index(rest, w)
		if i < 0 {
			t.Fatalf("registrations() = %q, want %q in this order, with new events anywhere", got, single)
		}
		rest = rest[i+1:]
	}
}

func TestRegisteringTheSameTypeTwicePanics(t *testing.T) {
	t.Parallel()
	mustPanic(t, "events: widget.bumped registered twice", func() {
		newRegistry([]Registration{
			Register[widgetBumped]("widget.bumped", 1),
			Register[widgetBumped]("widget.bumped", 2),
		})
	})
}

func TestRegisterRejectsATypeTheEventDoesNotReport(t *testing.T) {
	t.Parallel()
	mustPanic(t, "events: widget.bumped registered as widget.poked", func() {
		Register[widgetBumped]("widget.poked", 1)
	})
}

type widgetTicked struct {
	V int `json:"v"`
}

func (widgetTicked) Type() Type { return "widget.ticked" }

func (widgetTicked) core() {}

func TestRegisterCoreChecksTheTypeAndVersionLikeRegister(t *testing.T) {
	t.Parallel()
	mustPanic(t, "events: widget.ticked registered as widget.poked", func() {
		RegisterCore[widgetTicked]("widget.poked", 1)
	})
	mustPanic(t, "events: widget.ticked registered at version 0", func() {
		RegisterCore[widgetTicked]("widget.ticked", 0)
	})
}

func TestRegisterRejectsAVersionBelowOne(t *testing.T) {
	t.Parallel()
	mustPanic(t, "events: widget.bumped registered at version 0", func() {
		Register[widgetBumped]("widget.bumped", 0)
	})
}

func TestDecodeAcceptsCurrentAndPreviousVersionOnly(t *testing.T) {
	t.Parallel()
	r := newRegistry([]Registration{Register[widgetBumped]("widget.bumped", 3)})
	for _, tc := range []struct {
		v       int
		payload string
		ok      bool
	}{
		{3, `{"v":3,"id":"01890a5d-ac96-774b-bcce-b302099a8057","n":7}`, true},
		{2, `{"v":2,"id":"01890a5d-ac96-774b-bcce-b302099a8057","n":7}`, true},
		{1, `{"v":1,"id":"01890a5d-ac96-774b-bcce-b302099a8057","n":7}`, false},
		{4, `{"v":4,"id":"01890a5d-ac96-774b-bcce-b302099a8057","n":7}`, false},
	} {
		ev, err := r.decode("widget.bumped", tc.v, []byte(tc.payload))
		if !tc.ok {
			if errs.CodeOf(err) != errs.CodeDecodeFailed || ev != nil {
				t.Fatalf("v%d: decode = %v, %v; want nil and decode_failed", tc.v, ev, err)
			}
			continue
		}
		got, isWidget := ev.(widgetBumped)
		if err != nil || !isWidget || got.V != tc.v || got.N != 7 ||
			got.AggregateID().String() != "01890a5d-ac96-774b-bcce-b302099a8057" {
			t.Fatalf("v%d: decode = %#v, %v", tc.v, ev, err)
		}
	}
}

func TestDecodeFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		typ     Type
		v       int
		payload string
	}{
		{"unknown type", "nope.nothing", 1, `{"v":1}`},
		{"version zero", TypeSystemPinged, 0, `{"v":0}`},
		{"garbage", TypeSystemPinged, 1, "\x00garbage"},
		{"null", TypeSystemPinged, 1, `null`},
		{"missing v", TypeSystemPinged, 1, `{"note":"hi"}`},
		{"payload v disagrees", TypeSystemPinged, 1, `{"v":2}`},
		{"bad field", TypeSystemPinged, 1, `{"v":1,"ping_id":"not-a-uuid"}`},
		{"core subject", TypePriceTick, 1, `{"v":1,"as_of":"2026-03-01T12:00:00Z","prices":[]}`},
	} {
		ev, err := Decode(tc.typ, tc.v, []byte(tc.payload))
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeDecodeFailed || ev != nil {
			t.Fatalf("%s: Decode = %v, %v; want nil and *errs.Error decode_failed", tc.name, ev, err)
		}
	}
}

func TestCatalog(t *testing.T) {
	t.Parallel()
	got := Catalog()
	types := make([]Type, 0, len(got))
	for _, e := range got {
		types = append(types, e.Type)
	}
	if want := []Type{
		TypeAssetPriceMoved,
		TypeCabalAccessDecided, TypeCabalAccessRequested, TypeCabalCreated, TypeCabalMemberJoined,
		TypeCabalMemberLeft, TypeCabalUpdated, TypeDepositCredited, TypeFollowCreated, TypeFollowRemoved, TypePriceTick,
		TypeProposalCreated, TypeProposalExecuted, TypeProposalExecutionBlocked, TypeProposalExpired,
		TypeProposalFailed, TypeProposalPassed, TypeProposalVoided, TypeProposalWithdrawn,
		TypeSystemPinged, TypeTradeBlocked, TypeTradeConfirmed, TypeTradeFailed, TypeTradeSubmitted,
		TypeUserAuthStateChanged, TypeUserCreated, TypeUserDeleted, TypeUserNudgeDue, TypeUserProfileUpdated,
	}; !slices.Equal(types, want) {
		t.Fatalf("Catalog() types = %q, want %q", types, want)
	}
	e := got[slices.Index(types, TypeSystemPinged)]
	checkAssetPriceMoved(t, got[slices.Index(types, TypeAssetPriceMoved)])
	checkPriceTick(t, got[slices.Index(types, TypePriceTick)])
	want := []Field{{"v", "int"}, {"ping_id", "uuid.UUID"}, {"user_id", "uuid.UUID"}, {"note", "string"}}
	if e.Type != TypeSystemPinged || e.Subject != "events.system.pinged" || e.Core || e.Version != 1 ||
		!slices.Equal(e.Fields, want) {
		t.Fatalf("Catalog() system.pinged = %+v", e)
	}
}

func checkAssetPriceMoved(t *testing.T, moved Entry) {
	t.Helper()
	fields := []Field{
		{"v", "int"},
		{"asset_id", "uuid.UUID"},
		{"symbol", "string"},
		{"asset_name", "string"},
		{"threshold_bps", "int64"},
		{"change_bps", "int64"},
		{"mark_micros", "money.Micros"},
		{"prev_close_micros", "money.Micros"},
		{"trading_day", "string"},
		{"observed_at", "time.Time"},
	}
	if moved.Type != TypeAssetPriceMoved || moved.Subject != "events.asset.price_moved" || moved.Core ||
		moved.Version != 1 || !slices.Equal(moved.Fields, fields) {
		t.Fatalf("Catalog() asset.price_moved = %+v", moved)
	}
}

func checkPriceTick(t *testing.T, tick Entry) {
	t.Helper()
	tickFields := []Field{{"v", "int"}, {"as_of", "time.Time"}, {"prices", "[]events.TickPrice"}}
	if tick.Type != TypePriceTick || tick.Subject != "price.tick" || !tick.Core || tick.Version != 1 ||
		!slices.Equal(tick.Fields, tickFields) {
		t.Fatalf("Catalog() price.tick = %+v, want the core price.tick", tick)
	}
}

func TestPriceTickIsCore(t *testing.T) {
	t.Parallel()
	PriceTick{}.core()
}

func TestAssetPriceMovedAggregatesOnTheAsset(t *testing.T) {
	t.Parallel()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8057")
	if err != nil {
		t.Fatal(err)
	}
	var ev Event = AssetPriceMoved{V: 1, AssetID: id}
	if ev.Type() != TypeAssetPriceMoved || ev.AggregateType() != "asset" || ev.AggregateID() != id {
		t.Fatalf("AssetPriceMoved aggregate = %s %s %s", ev.Type(), ev.AggregateType(), ev.AggregateID())
	}
}

func TestSystemPingedAggregate(t *testing.T) {
	t.Parallel()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8057")
	if err != nil {
		t.Fatal(err)
	}
	var ev Event = SystemPinged{V: 1, PingID: id}
	if ev.Type() != TypeSystemPinged || ev.AggregateType() != "system" || ev.AggregateID() != id {
		t.Fatalf("SystemPinged aggregate = %s %s %s", ev.Type(), ev.AggregateType(), ev.AggregateID())
	}
}

func TestTradeEventAggregates(t *testing.T) {
	t.Parallel()
	swap, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8057")
	if err != nil {
		t.Fatal(err)
	}
	source, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8058")
	if err != nil {
		t.Fatal(err)
	}
	src := TradeSource{Kind: "proposal", ID: source}
	for _, tc := range []struct {
		ev       Event
		typ      Type
		aggType  string
		aggregID uuid.UUID
	}{
		{TradeBlocked{V: 1, Source: src}, TypeTradeBlocked, "proposal", source},
		{TradeSubmitted{V: 1, SwapID: swap, Source: src}, TypeTradeSubmitted, "swap", swap},
		{TradeConfirmed{V: 1, SwapID: swap, Source: src}, TypeTradeConfirmed, "swap", swap},
		{TradeFailed{V: 1, SwapID: swap, Source: src}, TypeTradeFailed, "swap", swap},
	} {
		if tc.ev.Type() != tc.typ || tc.ev.AggregateType() != tc.aggType || tc.ev.AggregateID() != tc.aggregID {
			t.Errorf("%T aggregate = %s %s %s, want %s %s %s", tc.ev, tc.ev.Type(), tc.ev.AggregateType(),
				tc.ev.AggregateID(), tc.typ, tc.aggType, tc.aggregID)
		}
	}
}

func TestProposalEventsAggregateOnTheProposal(t *testing.T) {
	t.Parallel()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8060")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []Event{
		ProposalCreated{ProposalID: id},
		ProposalPassed{ProposalID: id},
		ProposalFailed{ProposalID: id},
		ProposalExpired{ProposalID: id},
		ProposalWithdrawn{ProposalID: id},
		ProposalVoided{ProposalID: id},
		ProposalExecuted{ProposalID: id},
		ProposalExecutionBlocked{ProposalID: id},
	} {
		if ev.AggregateType() != "proposal" || ev.AggregateID() != id ||
			!strings.HasPrefix(string(ev.Type()), "proposal.") {
			t.Errorf("%T aggregate = %s %s %s", ev, ev.Type(), ev.AggregateType(), ev.AggregateID())
		}
	}
}

func TestFollowEventsAggregateOnTheFollow(t *testing.T) {
	t.Parallel()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8062")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []Event{FollowCreated{FollowID: id}, FollowRemoved{FollowID: id}} {
		if ev.AggregateType() != "follow" || ev.AggregateID() != id ||
			!strings.HasPrefix(string(ev.Type()), "follow.") {
			t.Errorf("%T aggregate = %s %s %s", ev, ev.Type(), ev.AggregateType(), ev.AggregateID())
		}
	}
}

func TestUserEventsAggregateOnTheUser(t *testing.T) {
	t.Parallel()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8061")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []Event{
		UserCreated{UserID: id},
		UserAuthStateChanged{UserID: id},
		UserProfileUpdated{UserID: id},
		UserDeleted{UserID: id},
		UserNudgeDue{UserID: id},
	} {
		if ev.AggregateType() != "user" || ev.AggregateID() != id || !strings.HasPrefix(string(ev.Type()), "user.") {
			t.Errorf("%T aggregate = %s %s %s", ev, ev.Type(), ev.AggregateType(), ev.AggregateID())
		}
	}
}
