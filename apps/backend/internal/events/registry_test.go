package events

import (
	"errors"
	"slices"
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
	if got, want := Subjects(), []string{"events.system.pinged"}; !slices.Equal(got, want) {
		t.Fatalf("Subjects() = %q, want %q", got, want)
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
	if len(got) != 1 {
		t.Fatalf("Catalog() has %d entries, want 1", len(got))
	}
	e := got[0]
	want := []Field{{"v", "int"}, {"ping_id", "uuid.UUID"}, {"user_id", "uuid.UUID"}, {"note", "string"}}
	if e.Type != TypeSystemPinged || e.Subject != "events.system.pinged" || e.Version != 1 ||
		!slices.Equal(e.Fields, want) {
		t.Fatalf("Catalog()[0] = %+v", e)
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
