package eventlog_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
