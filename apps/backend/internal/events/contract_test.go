package events_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
)

const goldenDir = "testdata/golden"

func golden() fs.FS { return os.DirFS(goldenDir) }

func fixtures(t *testing.T) map[events.Type]events.Event {
	t.Helper()
	id, err := uuid.Parse("01890a5d-ac96-774b-bcce-b302099a8057")
	if err != nil {
		t.Fatal(err)
	}
	return map[events.Type]events.Event{
		events.TypeSystemPinged: events.SystemPinged{V: 1, PingID: id, Note: "reference flow"},
	}
}

func goldenName(t events.Type, v int) string { return fmt.Sprintf("%s.v%d.json", t, v) }

func TestGoldenPayloads(t *testing.T) {
	t.Parallel()
	fx := fixtures(t)
	for _, entry := range events.Catalog() {
		ev, ok := fx[entry.Type]
		if !ok {
			t.Errorf("%s has no fixture", entry.Type)
			continue
		}
		got, err := json.MarshalIndent(ev, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		name := goldenName(entry.Type, entry.Version)
		if *update {
			if err := os.WriteFile(filepath.Join(goldenDir, name), got, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := fs.ReadFile(golden(), name)
		if err != nil {
			t.Fatalf("%s: %v (run go test ./internal/events -update)", entry.Type, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s payload drifted from %s\n got: %s\nwant: %s", entry.Type, name, got, want)
		}
	}
}

func TestEveryGoldenFileDecodesAsARegisteredVersion(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(golden(), "*.json")
	if err != nil {
		t.Fatal(err)
	}
	current := map[events.Type]int{}
	for _, entry := range events.Catalog() {
		current[entry.Type] = entry.Version
	}
	name := regexp.MustCompile(`^(.+)\.v([0-9]+)\.json$`)
	for _, path := range files {
		m := name.FindStringSubmatch(path)
		if m == nil {
			t.Errorf("%s is not named <type>.v<n>.json", path)
			continue
		}
		typ := events.Type(m[1])
		v, _ := strconv.Atoi(m[2])
		if _, ok := current[typ]; !ok {
			t.Errorf("%s has no registered type", path)
			continue
		}
		payload, err := fs.ReadFile(golden(), path)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := events.Decode(typ, v, payload)
		if err != nil || ev.Type() != typ {
			t.Errorf("%s: Decode = %v, %v", path, ev, err)
		}
		delete(current, typ)
	}
	for typ := range current {
		t.Errorf("%s has no golden file", typ)
	}
}

func FuzzDecode(f *testing.F) {
	for _, entry := range events.Catalog() {
		payload, err := fs.ReadFile(golden(), goldenName(entry.Type, entry.Version))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(entry.Type), entry.Version, payload)
	}
	f.Fuzz(func(t *testing.T, typ string, v int, payload []byte) {
		ev, err := events.Decode(events.Type(typ), v, payload)
		if err != nil {
			var e *errs.Error
			if !errors.As(err, &e) || e.Code != errs.CodeDecodeFailed || ev != nil {
				t.Fatalf("Decode(%q, %d) = %v, %v; want nil and decode_failed", typ, v, ev, err)
			}
			return
		}
		if ev.Type() != events.Type(typ) {
			t.Fatalf("Decode(%q, %d) returned %s", typ, v, ev.Type())
		}
	})
}
