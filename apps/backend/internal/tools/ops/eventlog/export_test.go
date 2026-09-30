package eventlog_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/eventlog"
)

const (
	scenario = "../../../testkit/scenarios/one-user-with-ping.jsonl"
	user     = "01890a5d-ac96-774b-bcce-b302099a8058"
	ping     = "01890a5d-ac96-774b-bcce-b302099a8057"
)

func export(t *testing.T, pool *pgxpool.Pool, o eventlog.Options) string {
	t.Helper()
	var out bytes.Buffer
	if err := eventlog.Export(t.Context(), pool, &out, o); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestExport_writesSeedLinesThatSeedBackIdentically(t *testing.T) {
	t.Parallel()
	want, err := os.ReadFile(scenario)
	if err != nil {
		t.Fatal(err)
	}
	source := testkit.DB(t)
	testkit.Seed(t, source, "one-user-with-ping")
	got := export(t, source, eventlog.Options{})
	if !jsonLinesEqual(got, string(want)) {
		t.Fatalf("export =\n%s\nwant the seeded scenario\n%s", got, want)
	}
	t.Run("seed the export", func(t *testing.T) {
		t.Parallel()
		copied := testkit.DB(t)
		testkit.SeedJSONL(t, copied, "exported", []byte(got))
		if again := export(t, copied, eventlog.Options{}); again != got {
			t.Fatalf("export after seeding the export =\n%s\nwant\n%s", again, got)
		}
	})
}

func jsonLinesEqual(a, b string) bool {
	x, y := strings.Split(strings.TrimSpace(a), "\n"), strings.Split(strings.TrimSpace(b), "\n")
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		var p, q any
		if json.Unmarshal([]byte(x[i]), &p) != nil || json.Unmarshal([]byte(y[i]), &q) != nil {
			return false
		}
		pj, _ := json.Marshal(p)
		qj, _ := json.Marshal(q)
		if !bytes.Equal(pj, qj) {
			return false
		}
	}
	return true
}

func TestExport_anonymizeHashesActorsAndPIIFieldsDeterministically(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	testkit.Seed(t, pool, "one-user-with-ping")
	got := export(t, pool, eventlog.Options{Anonymize: true})
	if again := export(t, pool, eventlog.Options{Anonymize: true}); again != got {
		t.Fatalf("anonymized exports differ:\n%s\n%s", got, again)
	}
	var line eventlog.Line
	if err := json.Unmarshal([]byte(got), &line); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(line.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	hashed := eventlog.Pseudonym(user)
	if strings.Contains(got, user) || strings.Contains(got, "seeded") || line.Actor != "user:"+hashed ||
		payload["user_id"] != hashed || payload["note"] != eventlog.Pseudonym("seeded") || payload["ping_id"] != ping {
		t.Fatalf("anonymized line = %s, want the user id and note hashed and the ping id kept", got)
	}
	if _, err := uuid.Parse(hashed); err != nil || !strings.HasPrefix(eventlog.Pseudonym("seeded"), "anon-") {
		t.Fatalf(
			"pseudonyms %s and %s, want a uuid for a uuid and anon- for text",
			hashed,
			eventlog.Pseudonym("seeded"),
		)
	}
	t.Run("seed the anonymized export", func(t *testing.T) {
		t.Parallel()
		testkit.SeedJSONL(t, testkit.DB(t), "anonymized", []byte(got))
	})
}

func cabalLine(t *testing.T, ev events.Event, actor string) []byte {
	t.Helper()
	payload, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(eventlog.Line{
		ID: uuid.NewSHA1(uuid.NameSpaceURL, payload), Type: ev.Type(), Actor: "user:" + actor,
		CreatedAt: time.Unix(1_800_000_000, 0).UTC(), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func pseudonymOf(t *testing.T, u uuid.UUID) uuid.UUID {
	t.Helper()
	return mustUUID(t, eventlog.Pseudonym(u.String()))
}

func decodeExported(t *testing.T, line string) events.Event {
	t.Helper()
	var l eventlog.Line
	if err := json.Unmarshal([]byte(line), &l); err != nil {
		t.Fatal(err)
	}
	ev, err := events.Decode(l.Type, 1, l.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestExport_anonymizeReachesUserIDsInsideANestedChangeList(t *testing.T) {
	t.Parallel()
	cabal, creator, member := mustUUID(t, ping), mustUUID(t, user), mustUUID(t, "01890a5d-ac96-774b-bcce-b302099a805b")
	name, bps := "Work pot", int32(250)
	changes := events.CabalChanges{Name: &name, SlippageBps: &bps}
	updated := events.CabalUpdated{V: 1, CabalID: cabal, ActorID: creator, Changes: changes}
	updated.Changes.VoterIDs = []uuid.UUID{creator, member}
	pool := testkit.DB(t)
	testkit.SeedJSONL(t, pool, "cabal updated", cabalLine(t, updated, user))
	got := export(t, pool, eventlog.Options{Anonymize: true})
	for _, real := range []uuid.UUID{creator, member} {
		if strings.Contains(got, real.String()) {
			t.Fatalf("anonymized export still holds %s:\n%s", real, got)
		}
	}
	want := events.CabalUpdated{V: 1, CabalID: cabal, ActorID: pseudonymOf(t, creator), Changes: changes}
	want.Changes.VoterIDs = []uuid.UUID{pseudonymOf(t, creator), pseudonymOf(t, member)}
	if back := decodeExported(t, got); !reflect.DeepEqual(back, want) {
		t.Fatalf("anonymized event = %+v, want %+v: voters and actor pseudonymized, the rest kept", back, want)
	}
	t.Run("seed the anonymized export", func(t *testing.T) {
		t.Parallel()
		testkit.SeedJSONL(t, testkit.DB(t), "anonymized cabal updated", []byte(got))
	})
}

func TestExport_anonymizeKeepsATimeFieldAndAnAbsentIDAndPseudonymizesTheScalarIDsBesideThem(t *testing.T) {
	t.Parallel()
	cabal, creator, member := mustUUID(t, ping), mustUUID(t, user), mustUUID(t, "01890a5d-ac96-774b-bcce-b302099a805b")
	request := mustUUID(t, "01890a5d-ac96-774b-bcce-b302099a805c")
	requested := events.CabalAccessRequested{
		V: 1, RequestID: request, CabalID: cabal, UserID: member, Direction: "invite", ActorID: creator,
		ExpiresAt: time.Unix(1_800_600_000, 0).UTC(),
	}
	expired := events.CabalAccessDecided{
		V: 1, RequestID: request, CabalID: cabal, UserID: member, Direction: "invite", Decision: "expired",
	}
	pool := testkit.DB(t)
	testkit.SeedJSONL(t, pool, "cabal access", append(cabalLine(t, requested, user), cabalLine(t, expired, user)...))
	lines := strings.Split(strings.TrimSpace(export(t, pool, eventlog.Options{Anonymize: true})), "\n")
	wantRequested, wantExpired := requested, expired
	wantRequested.UserID, wantRequested.ActorID = pseudonymOf(t, member), pseudonymOf(t, creator)
	wantExpired.UserID = pseudonymOf(t, member)
	if len(lines) != 2 {
		t.Fatalf("export has %d lines, want 2", len(lines))
	}
	for i, want := range []events.Event{wantRequested, wantExpired} {
		if back := decodeExported(t, lines[i]); !reflect.DeepEqual(back, want) {
			t.Errorf("line %d = %+v, want %+v", i, back, want)
		}
	}
}

func TestExport_filtersByAggregate(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	testkit.Seed(t, pool, "one-user-with-ping")
	id, err := uuid.Parse(ping)
	if err != nil {
		t.Fatal(err)
	}
	if got := export(
		t,
		pool,
		eventlog.Options{AggregateType: "system", AggregateID: id},
	); strings.Count(
		got,
		"\n",
	) != 1 {
		t.Fatalf("export of the ping's aggregate = %q, want its one event", got)
	}
	if got := export(t, pool, eventlog.Options{AggregateType: "system", AggregateID: uuid.Nil}); got != "" {
		t.Fatalf("export of another aggregate = %q, want nothing", got)
	}
}

func TestExport_failsOnAnUnreadableLogAnUndecodablePayloadOrAFailedWrite(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	testkit.Seed(t, pool, "one-user-with-ping")
	if err := eventlog.Export(
		t.Context(),
		pool,
		failingWriter{},
		eventlog.Options{},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("export into a failing writer = %v, want internal", err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE events SET payload = '{"v": 9}'`); err != nil {
		t.Fatal(err)
	}
	if err := eventlog.Export(t.Context(), pool, &bytes.Buffer{}, eventlog.Options{Anonymize: true}); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("anonymized export of an undecodable payload = %v, want decode_failed", err)
	}
	pool.Close()
	if err := eventlog.Export(
		t.Context(),
		pool,
		&bytes.Buffer{},
		eventlog.Options{},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("export from a closed pool = %v, want internal", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errs.New(errs.CodeInternal, "fixture.write")
}
