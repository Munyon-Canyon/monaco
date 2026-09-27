package migrations_test

import (
	"os"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/migrations"
)

func TestLatest_isTheHighestVersionPrefixOnDisk(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, e := range entries {
		if version, _, ok := strings.Cut(e.Name(), "_"); ok && strings.HasSuffix(e.Name(), ".sql") {
			want = max(want, version)
		}
	}
	if got := migrations.Latest(); got != want || got == "" {
		t.Fatalf("Latest() = %q, want %q from the migrations directory", got, want)
	}
}
