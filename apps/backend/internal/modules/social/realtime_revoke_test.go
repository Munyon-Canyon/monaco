package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f chatRoutes) connect(t *testing.T, user ids.UserID) *testkit.RealtimeClient {
	t.Helper()
	token, err := f.token(t, user)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.rt.Connect(app.TokenRequest{ClientID: token.ClientId, Capability: token.Capability})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (f chatRoutes) revoker() adapters.RealtimeRevoke { return adapters.RealtimeRevoke{Realtime: f.rt} }

func (f chatRoutes) rejoin(t *testing.T, user ids.UserID) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at) VALUES ($1, $2, 'member', true, $3)`,
		f.cabal.ID.UUID(), user.UUID(), f.now); err != nil {
		t.Fatal(err)
	}
}

func (f chatRoutes) say(t *testing.T, body string) {
	t.Helper()
	f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: body})
}

func receivedMessages(c *testkit.RealtimeClient) int {
	n := 0
	for _, p := range c.Received() {
		if p.Name == app.EventMessageCreated {
			n++
		}
	}
	return n
}

func (f chatRoutes) leaveAndRevoke(t *testing.T, user ids.UserID) {
	t.Helper()
	f.leave(t, user)
	if _, err := f.revoker().Left(t.Context(), events.CabalMemberLeft{
		V: 1, CabalID: f.cabal.ID.UUID(), UserID: user.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRealtimeRevoke_aLeaverWhoKeepsTheOldTokenHearsNothingMore(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	leaver, stayer := f.connect(t, f.member(2)), f.connect(t, f.member(1))
	f.say(t, "before")
	f.leaveAndRevoke(t, f.member(2))
	f.say(t, "after")
	if got := receivedMessages(leaver); got != 1 {
		t.Fatalf("leaver heard %d messages, want only the one sent before leaving", got)
	}
	if got := receivedMessages(stayer); got != 2 {
		t.Fatalf("current member heard %d messages, want both", got)
	}
}

func TestRealtimeRevoke_aLeaverCannotMintANewCapability(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.leaveAndRevoke(t, f.member(2))
	if _, err := f.token(t, f.member(2)); errs.CodeOf(err) != errs.CodeNoRealtimeChannels {
		t.Fatalf("token after leaving = %v, want no_realtime_channels", err)
	}
}

func TestRealtimeRevoke_rejoiningMintsACapabilityThatWorks(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	old := f.connect(t, f.member(2))
	f.leaveAndRevoke(t, f.member(2))
	f.rejoin(t, f.member(2))
	fresh := f.connect(t, f.member(2))
	f.say(t, "welcome back")
	if receivedMessages(fresh) != 1 || receivedMessages(old) != 0 {
		t.Fatalf("fresh client heard %d and the old client %d, want 1 and 0",
			receivedMessages(fresh), receivedMessages(old))
	}
}

func TestRealtimeRevoke_otherMembersKeepTheirTokens(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	stayer := f.connect(t, f.member(1))
	f.leaveAndRevoke(t, f.member(2))
	f.say(t, "still here")
	got, revoked := receivedMessages(stayer), f.rt.Revocations()
	if got != 1 || len(revoked) != 1 || revoked[0] != f.member(2) {
		t.Fatalf("stayer heard %d, revoked %v; want 1 and only the leaver", got, revoked)
	}
}

func TestRealtimeRevoke_aStandingThatIsNotActiveRevokes(t *testing.T) {
	t.Parallel()
	for _, to := range []string{"banned", "suspended", "deleted"} {
		t.Run(to, func(t *testing.T) {
			t.Parallel()
			f := newChatRoutes(t)
			client := f.connect(t, f.member(2))
			if _, err := f.revoker().Standing(t.Context(), events.UserAccountStatusChanged{
				V: 1, UserID: f.member(2).UUID(), From: "active", To: to, Cause: "admin",
			}); err != nil {
				t.Fatal(err)
			}
			f.say(t, "after")
			if got := receivedMessages(client); got != 0 {
				t.Fatalf("a %s account's old client heard %d messages, want none", to, got)
			}
		})
	}
}

func TestRealtimeRevoke_reinstatingLeavesTokensAlone(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	client := f.connect(t, f.member(2))
	if _, err := f.revoker().Standing(t.Context(), events.UserAccountStatusChanged{
		V: 1, UserID: f.member(2).UUID(), From: "banned", To: "active", Cause: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	f.say(t, "hello")
	if receivedMessages(client) != 1 || len(f.rt.Revocations()) != 0 {
		t.Fatalf("heard %d, revocations %v; want 1 and none", receivedMessages(client), f.rt.Revocations())
	}
}

func TestRealtimeRevoke_aDeletedAccountRevokes(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	client := f.connect(t, f.member(2))
	if _, err := f.revoker().Deleted(t.Context(), events.UserDeleted{
		V: 1, UserID: f.member(2).UUID(), At: f.now,
	}); err != nil {
		t.Fatal(err)
	}
	f.say(t, "after")
	if got := receivedMessages(client); got != 0 {
		t.Fatalf("a deleted account's old client heard %d messages, want none", got)
	}
}

func TestRealtimeRevoke_aFailedRevocationIsRetryableAndTheRetryRevokes(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	client := f.connect(t, f.member(2))
	f.rt.FailRevokeOnce(errs.New(errs.CodeUpstreamUnavailable, "test"))
	e := events.CabalMemberLeft{V: 1, CabalID: f.cabal.ID.UUID(), UserID: f.member(2).UUID()}
	f.leave(t, f.member(2))
	_, err := f.revoker().Left(t.Context(), e)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || errs.VerdictFor(errs.CodeOf(err)) != errs.VerdictNak {
		t.Fatalf("first attempt = %v (%s), want a retryable upstream_unavailable", err, errs.CodeOf(err))
	}
	f.say(t, "between")
	if _, err := f.revoker().Left(t.Context(), e); err != nil {
		t.Fatal(err)
	}
	f.say(t, "after")
	if got := receivedMessages(client); got != 1 {
		t.Fatalf("leaver heard %d messages, want only the one before the retry revoked", got)
	}
}

func TestRealtimeRevoke_isRegisteredOnItsOwnDurable(t *testing.T) {
	t.Parallel()
	for _, consumer := range social.New(module.Deps{}).Consumers() {
		if consumer.Durable != "social_realtime_revoke" {
			continue
		}
		got := map[string]events.Type{}
		for _, h := range consumer.Handlers {
			got[h.Name] = h.Type()
		}
		want := map[string]events.Type{
			"social.realtime_revoke.left":     events.TypeCabalMemberLeft,
			"social.realtime_revoke.standing": events.TypeUserAccountStatusChanged,
			"social.realtime_revoke.deleted":  events.TypeUserDeleted,
		}
		if len(got) != len(want) {
			t.Fatalf("handlers = %v, want %v", got, want)
		}
		for name, typ := range want {
			if got[name] != typ {
				t.Fatalf("handlers = %v, want %v", got, want)
			}
		}
		return
	}
	t.Fatal("social_realtime_revoke is not registered")
}
