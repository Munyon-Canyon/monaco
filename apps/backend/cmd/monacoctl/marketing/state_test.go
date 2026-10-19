package marketing_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func openStore(t *testing.T) (*marketing.Store, string) {
	t.Helper()
	repo := t.TempDir()
	s, err := marketing.OpenStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, repo
}

func TestRoll_ResetsTheDayCountersAtUTCMidnight(t *testing.T) {
	t.Parallel()
	st := marketing.State{Day: "2026-10-18", Sent: 30, Spent: 900, Failures: 2}

	st.Roll(time.Date(2026, 10, 18, 23, 59, 0, 0, time.UTC))
	if st.Sent != 30 || st.Spent != 900 {
		t.Fatalf("counters moved within the day: %+v", st)
	}

	local := time.FixedZone("UTC+2", 2*60*60)
	st.Roll(time.Date(2026, 10, 19, 1, 30, 0, 0, local))
	if st.Sent != 30 || st.Day != "2026-10-18" {
		t.Fatalf("1:30 at UTC+2 is still the 18th in UTC: %+v", st)
	}

	st.Roll(time.Date(2026, 10, 19, 0, 0, 1, 0, time.UTC))
	if st.Day != "2026-10-19" || st.Sent != 0 || st.Spent != 0 || st.Failures != 2 {
		t.Fatalf("new day = %+v, want counters reset and failures kept", st)
	}
}

func TestRoll_ForgetsPostsOlderThanTheSearchWindow(t *testing.T) {
	t.Parallel()
	st := marketing.State{Seen: map[string]time.Time{
		"old":    noon().Add(-8 * 24 * time.Hour),
		"edge":   noon().Add(-7 * 24 * time.Hour),
		"recent": noon().Add(-time.Hour),
	}}

	st.Roll(noon())

	if _, ok := st.Seen["old"]; ok || len(st.Seen) != 2 {
		t.Fatalf("seen = %v, want old dropped and edge and recent kept", st.Seen)
	}
	if st.Cursors == nil {
		t.Fatal("cursors nil, want an empty map")
	}
}

func TestStore_KeepsStateBetweenRuns(t *testing.T) {
	t.Parallel()
	s, repo := openStore(t)
	st, err := s.Load(noon())
	if err != nil {
		t.Fatal(err)
	}
	if st.Day != "2026-10-18" || len(st.Cursors) != 0 || len(st.Seen) != 0 {
		t.Fatalf("first state = %+v, want empty for today", st)
	}
	st.Cursors["club"], st.Seen["1002"], st.Sent, st.Spent = "1002", noon(), 3, 41

	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}

	again, err := marketing.OpenStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)
	got, err := again.Load(noon().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got.Cursors["club"] != "1002" || !got.Seen["1002"].Equal(noon()) || got.Sent != 3 || got.Spent != 41 {
		t.Fatalf("reloaded state = %+v", got)
	}
}

func TestStore_RefusesACorruptStateFile(t *testing.T) {
	t.Parallel()
	s, repo := openStore(t)
	path := filepath.Join(repo, ".build", "marketing", "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.Load(noon())

	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want decode_failed so a bad file never resets the dedup", err)
	}
}

func TestStore_ReportsAnUnreadableStateFile(t *testing.T) {
	t.Parallel()
	s, repo := openStore(t)
	if err := os.Mkdir(filepath.Join(repo, ".build", "marketing", "state.json"), 0o750); err != nil {
		t.Fatal(err)
	}

	_, err := s.Load(noon())

	if errs.CodeOf(err) != errs.CodeStorageUnavailable {
		t.Fatalf("err = %v, want storage_unavailable", err)
	}
}

func TestStore_SaveReportsEachWayItCanFail(t *testing.T) {
	t.Parallel()
	for name, blocked := range map[string]string{"the temp file": "state.json.tmp", "the rename": "state.json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, repo := openStore(t)
			if err := os.Mkdir(filepath.Join(repo, ".build", "marketing", blocked), 0o750); err != nil {
				t.Fatal(err)
			}

			err := s.Save(marketing.State{})

			if errs.CodeOf(err) != errs.CodeStorageUnavailable {
				t.Fatalf("err = %v, want storage_unavailable", err)
			}
		})
	}
}

func TestOpenStore_ReportsEachWayItCanFail(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "nowhere")
	if _, err := marketing.OpenStore(missing); errs.CodeOf(err) != errs.CodeStorageUnavailable {
		t.Errorf("missing repo: err = %v, want storage_unavailable", err)
	}

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".build"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := marketing.OpenStore(repo); errs.CodeOf(err) != errs.CodeStorageUnavailable {
		t.Errorf(".build is a file: err = %v, want storage_unavailable", err)
	}
}

func TestStore_AppendsLines(t *testing.T) {
	t.Parallel()
	s, repo := openStore(t)

	for _, line := range []string{"one\n", "two\n"} {
		if err := s.Append(".build/marketing/alerts.jsonl", []byte(line)); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := fs.ReadFile(os.DirFS(filepath.Join(repo, ".build", "marketing")), "alerts.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "one\ntwo\n" {
		t.Fatalf("file = %q, want both lines in order", raw)
	}
	if err := s.Append(".build/marketing", []byte("x")); errs.CodeOf(err) != errs.CodeStorageUnavailable ||
		strings.Contains(err.Error(), "x\n") {
		t.Errorf("appending to a directory: err = %v, want storage_unavailable", err)
	}
}
