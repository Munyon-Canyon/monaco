package verify

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocker_leavesNoContainerAfterAPassingOrAFailingStack(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("starts a real Postgres container; runs in test-backend.sh's non-short pass")
	}
	if err := exec.CommandContext(t.Context(), "docker", "info").Run(); err != nil {
		t.Skipf("no docker daemon: %v", err)
	}
	if err := Docker("docker").ensureImage(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		api  func(Binaries) string
		fail bool
	}{
		{"passing", func(b Binaries) string { return b.API }, false},
		{"failing", func(Binaries) string { return "/nonexistent/api" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, "ok")
			o.Postgres, o.Docker = nil, "docker"
			o.Atlas = filepath.Join(o.Dir, "..", "..", ".bin", "atlas")
			o.Bins.API = tc.api(o.Bins)
			s, err := Up(t.Context(), o)
			if (err != nil) != tc.fail {
				t.Errorf("Up = %v, want failure %v", err, tc.fail)
			}
			if err := s.Down(t.Context()); err != nil {
				t.Errorf("Down: %v", err)
			}
			out, err := exec.CommandContext(t.Context(), "docker", "ps", "-a", "-q",
				"--filter", "label="+ContainerLabel+"="+s.RunID).Output()
			if err != nil || strings.TrimSpace(string(out)) != "" {
				t.Fatalf("docker ps after Down = %q, %v, want no container", out, err)
			}
		})
	}
}
