package ci_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	ecrPostgres    = "public.ecr.aws/docker/library/postgres:16-alpine"
	mirrorPostgres = "mirror.gcr.io/library/postgres:16-alpine"
)

// runPullImage runs pull-image.sh with a fake docker that logs its arguments and fails every pull
// of an image whose name contains failing.
func runPullImage(t *testing.T, failing string, args ...string) (int, string, string) {
	t.Helper()
	return runPullImageFailingWith(t, failing, "toomanyrequests: Rate exceeded", args...)
}

func runPullImageFailingWith(t *testing.T, failing, message string, args ...string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> " + calls + "\n" +
		"if [[ -n '" + failing + "' && \"$1\" == pull && \"$*\" == *'" + failing + "'* ]]; then\n" +
		"  echo '" + message + "' >&2\n  exit 1\nfi\n"
	writeExecutable(t, filepath.Join(dir, "bin", "docker"), script)
	cmd := exec.Command("bash", append([]string{"pull-image.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"), "RETRY_ATTEMPTS=2", "RETRY_SLEEP=0")
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(calls)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return code, string(log), string(out)
}

func TestPullImage_pullsOnlyFromECRWhenECRSucceeds(t *testing.T) {
	t.Parallel()
	code, log, _ := runPullImage(t, "", ecrPostgres)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if want := "pull -q " + ecrPostgres + "\n"; log != want {
		t.Fatalf("docker calls = %q, want %q", log, want)
	}
}

func TestPullImage_fallsBackToTheMirrorAndTagsTheECRName(t *testing.T) {
	t.Parallel()
	code, log, _ := runPullImage(t, "public.ecr.aws", ecrPostgres)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"pull -q " + mirrorPostgres + "\n", "tag " + mirrorPostgres + " " + ecrPostgres + "\n"} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker calls = %q, missing %q", log, want)
		}
	}
}

func TestPullImage_goesStraightToTheMirrorWhenECRDataLimitIsExceeded(t *testing.T) {
	t.Parallel()
	code, log, _ := runPullImageFailingWith(t, "public.ecr.aws", "Error response from daemon: error from registry: Data limit exceeded", ecrPostgres)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := strings.Count(log, "pull -q "+ecrPostgres+"\n"); got != 1 {
		t.Fatalf("ECR pulls = %d, want 1: %q", got, log)
	}
	for _, want := range []string{"pull -q " + mirrorPostgres + "\n", "tag " + mirrorPostgres + " " + ecrPostgres + "\n"} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker calls = %q, missing %q", log, want)
		}
	}
}

func TestPullImage_exitsOneWithBothErrorsWhenECRDataLimitIsExceededAndTheMirrorFails(t *testing.T) {
	t.Parallel()
	code, log, out := runPullImageFailingWith(t, "postgres", "Error response from daemon: error from registry: Data limit exceeded", ecrPostgres)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if got := strings.Count(out, "Data limit exceeded"); got < 3 {
		t.Fatalf("output %q names the registry error %d times, want the ECR error and the mirror error each in the summary", out, got)
	}
	if !strings.Contains(out, "pull-image: "+ecrPostgres+": ") || !strings.Contains(out, "pull-image: "+mirrorPostgres+": ") {
		t.Fatalf("output %q does not carry both errors", out)
	}
	if got := strings.Count(log, "pull -q "+ecrPostgres+"\n"); got != 1 {
		t.Fatalf("ECR pulls = %d, want 1: %q", got, log)
	}
	if strings.Contains(log, "tag ") {
		t.Fatalf("docker calls = %q, tagged an image that never pulled", log)
	}
}

func TestPullImage_skipsECRRetriesWhenECRRateLimits(t *testing.T) {
	t.Parallel()
	code, log, _ := runPullImageFailingWith(t, "public.ecr.aws", "toomanyrequests: Data limit exceeded", ecrPostgres)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := strings.Count(log, "pull -q "+ecrPostgres+"\n"); got != 1 {
		t.Fatalf("ECR pulls = %d, want 1: %q", got, log)
	}
	if want := "pull -q " + mirrorPostgres + "\n"; !strings.Contains(log, want) {
		t.Fatalf("docker calls = %q, missing %q", log, want)
	}
}

func TestPullImage_retriesECRBeforeTheMirrorOnOtherFailures(t *testing.T) {
	t.Parallel()
	code, log, _ := runPullImageFailingWith(t, "public.ecr.aws", "connection reset by peer", ecrPostgres)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := strings.Count(log, "pull -q "+ecrPostgres+"\n"); got != 2 {
		t.Fatalf("ECR pulls = %d, want 2 (RETRY_ATTEMPTS): %q", got, log)
	}
	if want := "tag " + mirrorPostgres + " " + ecrPostgres + "\n"; !strings.Contains(log, want) {
		t.Fatalf("docker calls = %q, missing %q", log, want)
	}
}

func TestPullImage_failsNamingBothImagesWhenBothPullsFail(t *testing.T) {
	t.Parallel()
	code, log, out := runPullImage(t, "postgres", ecrPostgres)
	if code == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if !strings.Contains(out, ecrPostgres) || !strings.Contains(out, mirrorPostgres) {
		t.Fatalf("output %q does not name both images", out)
	}
	if strings.Contains(log, "tag ") {
		t.Fatalf("docker calls = %q, tagged an image that never pulled", log)
	}
}

func TestPullImage_refusesAnImageOutsideECRLibrary(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"postgres:16", "ghcr.io/daveshanley/vacuum:v0.30.6", "public.ecr.aws/docker/library/"} {
		code, log, _ := runPullImage(t, "", arg)
		if code == 0 || log != "" {
			t.Fatalf("%q: exit = %d, docker calls = %q, want a refusal before any docker call", arg, code, log)
		}
	}
}
