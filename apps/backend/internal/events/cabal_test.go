package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	cabalIDText  = "01890a5d-ac96-774b-bcce-b302099a8059"
	creatorText  = "01890a5d-ac96-774b-bcce-b302099a805a"
	memberText   = "01890a5d-ac96-774b-bcce-b302099a805b"
	requestText  = "01890a5d-ac96-774b-bcce-b302099a805c"
	cabalIDsHead = `"cabal_id":"` + cabalIDText + `"`
)

type cabalFixtures struct {
	cabalID, creator, member, request uuid.UUID
	created                           events.CabalCreated
	memberJoined                      events.CabalMemberJoined
	accessRequested                   events.CabalAccessRequested
	accessDecided                     events.CabalAccessDecided
	memberLeft                        events.CabalMemberLeft
	updated                           events.CabalUpdated
}

func parseUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ptr[T any](v T) *T { return &v }

func newCabalFixtures(t *testing.T) cabalFixtures {
	t.Helper()
	f := cabalFixtures{
		cabalID: parseUUID(t, cabalIDText), creator: parseUUID(t, creatorText),
		member: parseUUID(t, memberText), request: parseUUID(t, requestText),
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	f.created = events.CabalCreated{
		V: 1, CabalID: f.cabalID, CreatorID: f.creator, Name: "Friends pot", JoinMode: "request", VoterMode: "list",
		Threshold: "unanimous", ProposalExpirySeconds: 604800, SlippageBps: 50, TreasuryAddress: chain.AddressOf(key),
	}
	f.memberJoined = events.CabalMemberJoined{
		V: 1, CabalID: f.cabalID, UserID: f.member, Role: "member", Via: "invite", RequestID: f.request,
	}
	f.accessRequested = events.CabalAccessRequested{
		V: 1, RequestID: f.request, CabalID: f.cabalID, UserID: f.member, Direction: "invite", ActorID: f.creator,
		ExpiresAt: time.Unix(1_800_000_000, 0).UTC(),
	}
	f.accessDecided = events.CabalAccessDecided{
		V: 1, RequestID: f.request, CabalID: f.cabalID, UserID: f.member, Direction: "invite", Decision: "approved",
		ActorID: f.member,
	}
	f.memberLeft = events.CabalMemberLeft{V: 1, CabalID: f.cabalID, UserID: f.member, WasVoter: true}
	f.updated = events.CabalUpdated{
		V: 1, CabalID: f.cabalID, ActorID: f.creator, Changes: events.CabalChanges{
			Name: ptr("Work pot"), VoterMode: ptr("list"), VoterIDs: []uuid.UUID{f.creator, f.member},
			SlippageBps: ptr(int32(250)),
		},
	}
	return f
}

func (f cabalFixtures) all() []events.Event {
	return []events.Event{f.created, f.memberJoined, f.accessRequested, f.accessDecided, f.memberLeft, f.updated}
}

func TestCabalEvents_areAnnouncedOnTheCabalAggregateUnderTheirOwnType(t *testing.T) {
	t.Parallel()
	f := newCabalFixtures(t)
	wantTypes := []events.Type{
		events.TypeCabalCreated, events.TypeCabalMemberJoined, events.TypeCabalAccessRequested,
		events.TypeCabalAccessDecided, events.TypeCabalMemberLeft, events.TypeCabalUpdated,
	}
	for i, ev := range f.all() {
		if ev.Type() != wantTypes[i] || ev.AggregateType() != "cabal" || ev.AggregateID() != f.cabalID {
			t.Errorf("%T = %s on %s %s, want %s on cabal %s", ev, ev.Type(), ev.AggregateType(), ev.AggregateID(),
				wantTypes[i], f.cabalID)
		}
	}
}

func TestCabalEvents_leaveOutWhatTheirCommandDoesNotSet(t *testing.T) {
	t.Parallel()
	f := newCabalFixtures(t)
	user, actor, request := `"user_id":"`+memberText+`"`, `"actor_id":"`+creatorText+`"`, `"request_id":"`+requestText+`"`
	for name, tt := range map[string]struct {
		ev   events.Event
		want string
	}{
		"a creator's join has no request": {
			events.CabalMemberJoined{V: 1, CabalID: f.cabalID, UserID: f.member, Role: "creator", Via: "create"},
			`{"v":1,` + cabalIDsHead + `,` + user + `,"role":"creator","via":"create"}`,
		},
		"a request has no expiry": {
			events.CabalAccessRequested{
				V: 1, RequestID: f.request, CabalID: f.cabalID, UserID: f.member, Direction: "request", ActorID: f.creator,
			},
			`{"v":1,` + request + `,` + cabalIDsHead + `,` + user + `,"direction":"request",` + actor + `}`,
		},
		"an expiry has no actor": {
			events.CabalAccessDecided{
				V: 1, RequestID: f.request, CabalID: f.cabalID, UserID: f.member, Direction: "invite", Decision: "expired",
			},
			`{"v":1,` + request + `,` + cabalIDsHead + `,` + user + `,"direction":"invite","decision":"expired"}`,
		},
		"an update names only the fields it changed": {
			events.CabalUpdated{V: 1, CabalID: f.cabalID, ActorID: f.creator, Changes: events.CabalChanges{
				Threshold: ptr("unanimous"), ProposalExpirySeconds: ptr(int32(3600)),
			}},
			`{"v":1,` + cabalIDsHead + `,` + actor + `,"changes":{"threshold":"unanimous","proposal_expiry_seconds":3600}}`,
		},
		"a cleared picture is an empty string": {
			events.CabalUpdated{
				V: 1, CabalID: f.cabalID, ActorID: f.creator, Changes: events.CabalChanges{PictureURL: ptr("")},
			},
			`{"v":1,` + cabalIDsHead + `,` + actor + `,"changes":{"picture_url":""}}`,
		},
	} {
		got, err := json.Marshal(tt.ev)
		if err != nil || string(got) != tt.want {
			t.Errorf("%s: payload %s, %v; want %s", name, got, err, tt.want)
			continue
		}
		back, err := events.Decode(tt.ev.Type(), 1, got)
		again, errAgain := json.Marshal(back)
		if err != nil || errAgain != nil || string(again) != tt.want {
			t.Errorf("%s: decoded and re-encoded as %s, %v, %v; want %s", name, again, err, errAgain, tt.want)
		}
	}
}
