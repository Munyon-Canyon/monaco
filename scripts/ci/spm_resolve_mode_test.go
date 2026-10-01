package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPMResolveMode_exactKeyOnly(t *testing.T) {
	script := "spm-resolve-mode.sh"
	withFile := filepath.Join(t.TempDir(), "workspace-state.json")
	if err := os.WriteFile(withFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "workspace-state.json")

	cases := []struct {
		name  string
		hit   string
		fetch string
		state string
		want  string
	}{
		{name: "primary key and state file", hit: "true", state: withFile, want: "exact"},
		{name: "warm artifact and state file", fetch: "artifact run 36780876210", state: withFile, want: "exact"},
		{name: "primary key wins over a nothing fetch", hit: "true", fetch: "nothing", state: withFile, want: "exact"},
		{name: "state file from a prefix restore", hit: "false", state: withFile, want: "inexact"},
		{name: "state file and a failed fetch", hit: "false", fetch: "nothing", state: withFile, want: "inexact"},
		{name: "primary key without the state file", hit: "true", state: missing, want: "inexact"},
		{name: "artifact word without the run prefix", fetch: "artifact running 1", state: withFile, want: "inexact"},
		{name: "cold", state: missing, want: "inexact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", script)
			cmd.Env = append(os.Environ(),
				"SPM_HIT="+tc.hit,
				"SPM_FETCH="+tc.fetch,
				"STATE_FILE="+tc.state,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("mode: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
