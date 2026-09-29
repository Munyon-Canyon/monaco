package testkit_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMainCommand_turnsOffTheRaceDetectorExitSleepInTheChild(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"no GORACE", []string{"A=1"}, "GORACE=atexit_sleep_ms=0"},
		{"empty GORACE", []string{"GORACE="}, "GORACE=atexit_sleep_ms=0"},
		{"existing options", []string{"GORACE=halt_on_error=1"}, "GORACE=halt_on_error=1 atexit_sleep_ms=0"},
		{
			"the last GORACE wins",
			[]string{"GORACE=log_path=a", "GORACE=history_size=2"},
			"GORACE=history_size=2 atexit_sleep_ms=0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := slices.Clone(tc.env)
			got := testkit.MainCommand(t, env).Env
			var last string
			for _, kv := range got {
				if strings.HasPrefix(kv, "GORACE=") {
					last = kv
				}
			}
			if last != tc.want {
				t.Fatalf("child GORACE = %q in %q, want %q", last, got, tc.want)
			}
			if !slices.Equal(env, tc.env) {
				t.Fatalf("MainCommand changed the caller's env to %q", env)
			}
		})
	}
}
