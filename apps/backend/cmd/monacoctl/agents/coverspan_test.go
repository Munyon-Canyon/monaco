package agents

import (
	"os"
	"path/filepath"
	"testing"
)

const spanSource = `package x

var limit = 3

func A() int {
	n := limit
	return n
}

func B() int {
	return 2
}
`

func TestWidenToFuncs_judgesTheWholeFunctionAroundAChangedLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(spanSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package x\nfunc ("), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ in, want string }{
		"a line inside a function":      {"x.go:6-6\n", "x.go:5-8\n"},
		"a range across two functions":  {"x.go:7-11\n", "x.go:5-12\n"},
		"a line outside every function": {"x.go:3-3\n", "x.go:3-3\n"},
		"a file that does not parse":    {"broken.go:2-2\n", "broken.go:2-2\n"},
		"a file that is not on disk":    {"gone.go:4-5\n", "gone.go:4-5\n"},
		"no changed lines":              {"", ""},
	} {
		if got := widenToFuncs(dir, tc.in); got != tc.want {
			t.Errorf("%s: widenToFuncs(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}
