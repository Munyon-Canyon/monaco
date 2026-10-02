package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const publishPoll = 20 * time.Millisecond

func AsUser(name string) Step {
	return func(s *Scenario) { s.actor = s.user(name) }
}

func AsSeededUser(name string, id ids.UserID) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if u, ok := s.users[name]; ok && u.id == id {
			s.actor = u
			return
		}
		s.actor = s.addUser(name, id)
	}
}

func Anonymous() Step {
	return func(s *Scenario) { s.actor = nil }
}

func (s *Scenario) token() string {
	if s.actor == nil {
		return ""
	}
	return s.actor.token
}

func SignIn(sub string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodPost, path: "/v1/auth/session", token: s.app.privyToken(sub)})
		if s.last.status < http.StatusOK || s.last.status >= http.StatusMultipleChoices {
			return
		}
		var me struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(s.last.body, &me); err != nil {
			s.t.Fatalf("scenario: sign-in response %s: %v", s.last.body, err)
		}
		id, err := ids.ParseUserID(me.ID)
		if err != nil {
			s.t.Fatalf("scenario: sign-in response %s has no user id: %v", s.last.body, err)
		}
		if known, ok := s.users[sub]; ok && known.id == id {
			s.actor = known
			return
		}
		s.actor = s.addUser(sub, id)
	}
}

func FakeUpstream(step fakes.Step) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.app.script(s.t.Context(), s.t, step)
	}
}

func Post(path, body string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodPost, path: s.path(path), body: body, key: s.nextKey(), token: s.token()})
	}
}

func Patch(path, body string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodPatch, path: s.path(path), body: body, key: s.nextKey(), token: s.token()})
	}
}

func PostPhoto(path string, photo []byte) Step {
	return func(s *Scenario) {
		s.t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("photo", "photo.jpg")
		if err != nil {
			s.t.Fatalf("scenario: create photo form: %v", err)
		}
		if _, err := part.Write(photo); err != nil {
			s.t.Fatalf("scenario: write photo form: %v", err)
		}
		if err := form.Close(); err != nil {
			s.t.Fatalf("scenario: close photo form: %v", err)
		}
		s.send(request{
			method: http.MethodPost, path: s.path(path), body: body.String(), key: s.nextKey(), token: s.token(),
			contentType: form.FormDataContentType(),
		})
	}
}

func Delete(path string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodDelete, path: s.path(path), key: s.nextKey(), token: s.token()})
	}
}

func Put(path, body string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodPut, path: s.path(path), body: body, key: s.nextKey(), token: s.token()})
	}
}

func Get(path string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodGet, path: s.path(path), token: s.token()})
	}
}

func Replay() Step {
	return func(s *Scenario) {
		s.t.Helper()
		first := *s.response()
		s.send(first.req)
		if s.last.status != first.status || !equalJSON(s.last.body, first.body) {
			s.t.Fatalf("scenario: replaying %s %s with key %s answered %d %s, want the first answer %d %s",
				first.req.method, first.req.path, first.req.key, s.last.status, s.last.body, first.status, first.body)
		}
	}
}

func Retry() Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(s.response().req)
	}
}

func ExpectStatus(status int) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if r := s.response(); r.status != status {
			s.t.Fatalf("scenario: %s %s answered %d %s, want %d", r.req.method, r.req.path, r.status, r.body, status)
		}
	}
}

func ExpectProblem(code errs.Code) Step {
	return func(s *Scenario) {
		s.t.Helper()
		r := s.response()
		var p api.Problem
		if r.header.Get("Content-Type") != "application/problem+json" || json.Unmarshal(r.body, &p) != nil ||
			p.Code != api.ErrorCode(code) {
			s.t.Fatalf("scenario: %s %s answered %d %s, want problem %s",
				r.req.method, r.req.path, r.status, r.body, code)
		}
	}
}

func ExpectJSON(field string, want any) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if got := s.field(field); !equalJSON(got, mustMarshal(want)) {
			s.t.Fatalf("scenario: response field %s = %s, want %v", field, got, want)
		}
	}
}

func ExpectRemembered(field, name string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		var got string
		if err := json.Unmarshal(s.field(field), &got); err != nil || got != s.remember[name] {
			s.t.Fatalf("scenario: response field %s = %s, want the remembered %s %q", field, s.field(field), name,
				s.remember[name])
		}
	}
}

func Remember(field, as string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		var v string
		if err := json.Unmarshal(s.field(field), &v); err != nil {
			s.t.Fatalf("scenario: remember %s: %v", field, err)
		}
		s.remember[as] = v
	}
}

func ExpectEvents(typ events.Type, n int) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if got := s.app.events(s.t, typ, s.actors()); len(got) != n {
			s.t.Fatalf("scenario: %d %s events appended, want %d", len(got), typ, n)
		}
	}
}

func ExpectEventPayload(typ events.Type, want any) Step {
	return func(s *Scenario) {
		s.t.Helper()
		var wantFields map[string]any
		if err := json.Unmarshal(mustMarshal(want), &wantFields); err != nil {
			s.t.Fatalf("scenario: encode %s event payload expectation: %v", typ, err)
		}
		for _, payload := range s.app.eventPayloads(s.t, typ) {
			var got map[string]any
			if err := json.Unmarshal(payload, &got); err != nil {
				s.t.Fatalf("scenario: decode %s event payload: %v", typ, err)
			}
			if eventPayloadMatches(got, wantFields) {
				return
			}
		}
		s.t.Fatalf("scenario: no %s event payload matches %s", typ, mustMarshal(want))
	}
}

func eventPayloadMatches(got, want map[string]any) bool {
	for key, value := range want {
		if !reflect.DeepEqual(got[key], value) {
			return false
		}
	}
	return true
}

func EventuallyEvent(typ events.Type) Step {
	return func(s *Scenario) {
		s.t.Helper()
		appended := s.app.events(s.t, typ, s.actors())
		if len(appended) == 0 {
			s.t.Fatalf("scenario: no %s event was appended", typ)
		}
		s.app.awaitHandled(s.t, typ, appended)
	}
}

func EventuallyHint(what string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if s.actor == nil {
			s.t.Fatal("scenario: EventuallyHint needs AsUser first")
		}
		u := s.actor
		want := sse.Hint{Key: sse.UserKey(u.id), What: what}
		s.app.note.await(s.t, "hint "+string(want.Key)+" "+what, func() bool {
			return slices.Contains(u.stream.hints, want)
		})
	}
}

func HoldRelay() Step {
	return func(s *Scenario) { s.app.hold() }
}

func PublishCrashingAt(point faultpoint.Name) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.app.crashAt(s.t, point)
	}
}

func ExpectPublished(typ events.Type, n uint64) Step {
	return func(s *Scenario) {
		s.t.Helper()
		if got := s.app.published(s.t, typ, s.app.events(s.t, typ, s.actors())); got != n {
			s.t.Fatalf("scenario: %d %s messages on the stream, want %d", got, typ, n)
		}
	}
}

func EventuallyPublished(typ events.Type, n uint64) Step {
	return func(s *Scenario) {
		s.t.Helper()
		deadline := time.NewTimer(convergeWithin)
		defer deadline.Stop()
		tick := time.NewTicker(publishPoll)
		defer tick.Stop()
		for s.app.published(s.t, typ, s.app.events(s.t, typ, s.actors())) != n {
			select {
			case <-deadline.C:
				s.t.Fatalf("scenario: %d %s messages were not published within %s", n, typ, convergeWithin)
			case <-tick.C:
			case <-s.t.Context().Done():
				s.t.Fatalf("scenario: %d %s messages were not published: %v", n, typ, context.Cause(s.t.Context()))
			}
		}
	}
}

func SeededUser(name, status string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		u := s.user(name)
		id := u.id.String()
		handle := "u" + strings.ReplaceAll(id, "-", "")[13:]
		_, err := s.app.pool.Exec(s.t.Context(), `INSERT INTO users (id, privy_user_id, handle, login_provider,
			account_status, auth_state_changed_at, created_at, updated_at)
			VALUES ($1, $2, $3, 'sms', $4, now(), now(), now())`, id, "did:privy:"+id, handle, status)
		if err != nil {
			s.t.Fatalf("scenario: seed the user %s: %v", name, err)
		}
		s.remember[name] = id
	}
}

func Seeded(name string, users ...string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		seen := map[string]bool{}
		for _, ev := range s.app.seed(s.t, name) {
			s.remember[string(ev.Event.Type())] = ev.Event.AggregateID().String()
			kind, id, _ := strings.Cut(ev.Actor, ":")
			if kind != "user" || seen[id] || len(seen) == len(users) {
				continue
			}
			uid, err := ids.ParseUserID(id)
			if err != nil {
				s.t.Fatalf("scenario: seed %s actor %s: %v", name, ev.Actor, err)
			}
			s.addUser(users[len(seen)], uid)
			seen[id] = true
		}
	}
}
