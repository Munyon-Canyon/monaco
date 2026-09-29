package scenario

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func AsUser(name string) Step {
	return func(s *Scenario) { s.actor = s.user(name) }
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

func Post(path, body string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		s.send(request{method: http.MethodPost, path: s.path(path), body: body, key: s.nextKey(), token: s.token()})
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
		if got := s.events(typ); len(got) != n {
			s.t.Fatalf("scenario: %d %s events appended, want %d", len(got), typ, n)
		}
	}
}

func (s *Scenario) events(typ events.Type) []string {
	s.t.Helper()
	rows, err := s.app.pool.Query(s.t.Context(), `SELECT id::text FROM events WHERE type = $1`, string(typ))
	var got []string
	for err == nil && rows.Next() {
		var id string
		err = rows.Scan(&id)
		got = append(got, id)
	}
	if err == nil {
		rows.Close()
		err = rows.Err()
	}
	if err != nil {
		s.t.Fatalf("scenario: read %s events: %v", typ, err)
	}
	return got
}

func EventuallyEvent(typ events.Type) Step {
	return func(s *Scenario) {
		s.t.Helper()
		appended := s.events(typ)
		if len(appended) == 0 {
			s.t.Fatalf("scenario: no %s event was appended", typ)
		}
		handlers := s.app.handlersOf(typ)
		s.app.await(s.t, "every handler of "+string(typ)+" committing "+strings.Join(appended, ", "), func() bool {
			return s.app.handledAll(handlers, appended)
		})
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
		s.app.await(s.t, "hint "+string(want.Key)+" "+what, func() bool {
			for _, h := range u.stream.hints {
				if h == want {
					return true
				}
			}
			return false
		})
	}
}

func HoldRelay() Step {
	return func(s *Scenario) { s.app.held.Store(true) }
}

func PublishCrashingAt(point faultpoint.Name) Step {
	return func(s *Scenario) {
		s.t.Helper()
		testkit.CrashAt(s.t, point, func(ctx context.Context) error {
			s.app.relay.Once(ctx)
			return nil
		})
		s.app.held.Store(false)
	}
}

func ExpectPublished(typ events.Type, n uint64) Step {
	return func(s *Scenario) {
		s.t.Helper()
		stream, err := s.app.bus.JS.Stream(s.t.Context(), s.app.bus.Events)
		var info *jetstream.StreamInfo
		if err == nil {
			info, err = stream.Info(s.t.Context(), jetstream.WithSubjectFilter(s.app.bus.Conn.Subject(typ.Subject())))
		}
		if err != nil {
			s.t.Fatalf("scenario: read the events stream: %v", err)
		}
		if got := info.State.Subjects[s.app.bus.Conn.Subject(typ.Subject())]; got != n {
			s.t.Fatalf("scenario: %d %s messages on the stream, want %d", got, typ, n)
		}
	}
}

func Seeded(name string, users ...string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		seen := map[string]bool{}
		for _, ev := range testkit.Seed(s.t, s.app.pool, name, s.app.consumers...) {
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
