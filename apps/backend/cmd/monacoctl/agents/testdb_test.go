package agents

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTakeTestDB_failsWhenNoSlotCanBeTaken(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		setup func(t *testing.T, slots string)
		want  string
	}{
		"slot dir is a file": {
			setup: func(t *testing.T, slots string) { t.Helper(); writeFile(t, slots, "") },
			want:  "take a test database slot",
		},
		"first slot unopenable": {
			setup: func(t *testing.T, slots string) { t.Helper(); mkdir(t, filepath.Join(slots, "0")) },
			want:  "take a test database slot",
		},
		"a later slot unopenable": {
			setup: func(t *testing.T, slots string) { t.Helper(); mkdir(t, filepath.Join(slots, "5")) },
			want:  "count test database slots",
		},
		"every slot held": {
			setup: func(t *testing.T, slots string) {
				t.Helper()
				mkdir(t, slots)
				for slot := range testDBSlots {
					held, ok, err := tryLock(filepath.Join(slots, strconv.Itoa(slot)))
					if err != nil || !ok {
						t.Fatalf("hold slot %d: %v %v", slot, ok, err)
					}
					t.Cleanup(func() { _ = held.Close() })
				}
			},
			want: "every test database slot is held: 16 in ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := &Env{Common: t.TempDir(), Work: t.TempDir()}
			c.setup(t, filepath.Join(env.Common, ".monaco", "test-db"))
			if _, _, err := env.takeTestDB(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("takeTestDB() = %v, want %q", err, c.want)
			}
		})
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
}

func TestCheck_failsBeforeAnyRowWhenNoTestDatabaseSlotCanBeTaken(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	writeFile(t, filepath.Join(h.Env(t).Common, ".monaco", "test-db"), "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "take a test database slot") ||
		slices.ContainsFunc(h.calls, func(c string) bool { return !strings.Contains(c, "gt parent") }) {
		t.Fatalf("an unusable slot dir: %d %q %v", code, stderr, h.calls)
	}
}
