package main

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

var oapiCodegenState sync.Mutex

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithChild(main))
}

func TestMain_exitsWithTheRunCode(t *testing.T) {
	t.Parallel()
	var exit *exec.ExitError
	if err := testkit.MainCommand(t, os.Environ(), "nope").Run(); !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("gen nope = %v, want exit code 2", err)
	}
}

func TestEveryStepFailsOutsideTheBackendDir(t *testing.T) {
	t.Parallel()
	oapiCodegenState.Lock()
	defer oapiCodegenState.Unlock()
	for _, s := range steps(t.Context()) {
		var out strings.Builder
		if code := run(steps(t.Context()), []string{s.name}, &out, &out); code != 1 ||
			!strings.HasPrefix(out.String(), "gen "+s.name+": ") {
			t.Errorf("gen %s from cmd/gen = %d %q, want 1 and the step's error", s.name, code, out.String())
		}
	}
}

func fakeSteps(ran *[]string, fail string) []step {
	all := make([]step, 0, 3)
	for _, name := range []string{"a", "b", "c"} {
		all = append(all, step{name, func() error {
			*ran = append(*ran, name)
			if name == fail {
				return errors.New("boom")
			}
			return nil
		}})
	}
	return all
}

func TestRunRunsOneStepByNameOrEveryStepForAll(t *testing.T) {
	t.Parallel()
	const usage = "usage: go run ./cmd/gen all|a|b|c\n"
	for _, tc := range []struct {
		args []string
		code int
		ran  []string
		out  string
	}{
		{[]string{"all"}, 0, []string{"a", "b", "c"}, "gen a ok\ngen b ok\ngen c ok\n"},
		{[]string{"b"}, 0, []string{"b"}, "gen b ok\n"},
		{[]string{"nope"}, 2, nil, usage},
		{nil, 2, nil, usage},
		{[]string{"a", "b"}, 2, nil, usage},
	} {
		var ran []string
		var out strings.Builder
		code := run(fakeSteps(&ran, ""), tc.args, &out, &out)
		if code != tc.code || !slices.Equal(ran, tc.ran) || out.String() != tc.out {
			t.Errorf("run %v = %d, ran %v, output %q; want %d, %v, %q",
				tc.args, code, ran, out.String(), tc.code, tc.ran, tc.out)
		}
	}
}

func TestRunStopsAtTheFirstFailingStepAndNamesIt(t *testing.T) {
	t.Parallel()
	var ran []string
	var out strings.Builder
	code := run(fakeSteps(&ran, "b"), []string{"all"}, &out, &out)
	if code != 1 || !slices.Equal(ran, []string{"a", "b"}) || out.String() != "gen a ok\ngen b: boom\n" {
		t.Fatalf("run all = %d, ran %v, output %q", code, ran, out.String())
	}
}

func TestAllRunsEveryStepInOrder(t *testing.T) {
	t.Parallel()
	var ran []string
	all := steps(t.Context())
	names := make([]string, len(all))
	for i := range all {
		names[i] = all[i].name
		all[i].run = func() error { ran = append(ran, names[i]); return nil }
	}
	want := []string{"golangci", "registry", "sqlc", "errors", "openapi", "httpapi", "flows", "docs", "hash"}
	if !slices.Equal(names, want) {
		t.Fatalf("steps = %v, want %v", names, want)
	}
	var out strings.Builder
	if code := run(all, []string{"all"}, &out, &out); code != 0 || !slices.Equal(ran, want) {
		t.Fatalf("run all = %d, ran %v, output %q", code, ran, out.String())
	}
}

func TestQuietReturnsTheCommandOutputOnlyOnFailure(t *testing.T) {
	t.Parallel()
	if err := quiet(exec.CommandContext(t.Context(), "go", "version")); err != nil {
		t.Fatalf("go version: %v", err)
	}
	err := quiet(exec.CommandContext(t.Context(), "go", "nope"))
	if err == nil || !strings.Contains(err.Error(), "go nope") || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("go nope: %v", err)
	}
}
