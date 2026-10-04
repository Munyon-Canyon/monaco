package events_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	goldenDir = "testdata/golden"
	aaplxMint = chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
)

func golden() fs.FS { return os.DirFS(goldenDir) }

func goldenName(t events.Type, v int) string { return fmt.Sprintf("%s.v%d.json", t, v) }

func roundTrip(t *testing.T, typ reflect.Type, payload []byte) []byte {
	t.Helper()
	decoded := reflect.New(typ)
	if err := json.Unmarshal(payload, decoded.Interface()); err != nil {
		t.Fatalf("%s: %v", typ, err)
	}
	got, err := json.MarshalIndent(decoded.Elem().Interface(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(got, '\n')
}

func TestGoldenPayloads(t *testing.T) {
	t.Parallel()
	goTypes := events.GoTypes()
	for _, entry := range events.Catalog() {
		name := goldenName(entry.Type, entry.Version)
		want, err := fs.ReadFile(golden(), name)
		if errors.Is(err, fs.ErrNotExist) && *update {
			want = roundTrip(t, goTypes[entry.Type], fmt.Appendf(nil, `{"v":%d}`, entry.Version))
			if err := os.WriteFile(filepath.Join(goldenDir, name), want, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s from the zero value at v%d; fill in realistic values", name, entry.Version)
		} else if err != nil {
			t.Fatalf("%s: %v (run go test -short ./internal/events -update, then fill in real values)", entry.Type, err)
		}
		if got := roundTrip(t, goTypes[entry.Type], want); !bytes.Equal(got, want) {
			t.Errorf("%s does not round-trip through %s\n got: %s\nwant: %s", name, goTypes[entry.Type], got, want)
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
	core := map[events.Type]bool{}
	for _, entry := range events.Catalog() {
		current[entry.Type] = entry.Version
		core[entry.Type] = entry.Core
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
		if core[typ] {
			delete(current, typ)
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

func TestCorePayloadsRoundTripThroughTheirGoType(t *testing.T) {
	t.Parallel()
	goTypes := events.GoTypes()
	for _, entry := range events.Catalog() {
		if !entry.Core {
			continue
		}
		want, err := fs.ReadFile(golden(), goldenName(entry.Type, entry.Version))
		if err != nil {
			t.Fatal(err)
		}
		if got := roundTrip(t, goTypes[entry.Type], want); !bytes.Equal(got, want) {
			t.Errorf("%s did not survive a decode\n got: %s\nwant: %s", entry.Type, got, want)
		}
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
