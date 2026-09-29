package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeDocker(t *testing.T, port, rm, image string) (Docker, string) {
	t.Helper()
	calls := filepath.Join(t.TempDir(), "calls")
	return Docker(writeScript(t, "docker", `echo "$@" >> `+calls+`
case "$1" in
  run) echo 0123abcd ;;
  port) `+port+` ;;
  rm) `+rm+` ;;
  image) `+image+` ;;
  pull) echo "pull failed" >&2; exit 1 ;;
esac
`)), calls
}

func readCalls(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStartPostgres_runsATmpfsContainerOnARandomPortAndRemovesItByName(t *testing.T) {
	t.Parallel()
	d, calls := fakeDocker(t, `printf '127.0.0.1:49153\n[::]:49153\n'`, "exit 0", "exit 0")
	pg, err := startPostgres(t.Context(), d, "run1")
	if err != nil {
		t.Fatal(err)
	}
	if pg.url != "postgres://monaco:monaco@127.0.0.1:49153/monaco?sslmode=disable" {
		t.Fatalf("url = %q", pg.url)
	}
	if err := pg.remove(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := readCalls(t, calls)
	for _, want := range []string{
		"run -d --rm --name monaco-verify-run1 --label monaco.verify=run1 -p 127.0.0.1::5432 " +
			"--tmpfs /var/lib/postgresql/data",
		"postgres:16-alpine postgres -c fsync=off",
		"port monaco-verify-run1 5432/tcp",
		"rm -f -v monaco-verify-run1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("docker calls:\n%s\nwant %q", got, want)
		}
	}
	if strings.Contains(got, "monaco-postgres ") {
		t.Fatal("verify touched monaco-postgres")
	}
}

func TestStartPostgres_reportsEachDockerFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, port, want string
		run              bool
	}{
		{"run", "exit 0", "docker run: exit status 9: no daemon", false},
		{"port", `echo "no port" >&2; exit 1`, "docker port: exit status 1: no port", true},
		{"garbage", "echo garbage", `docker port monaco-verify-x printed "garbage"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, _ := fakeDocker(t, tc.port, "exit 0", "exit 0")
			if !tc.run {
				d = Docker(writeScript(t, "docker", `echo "no daemon" >&2; exit 9`))
			}
			pg, err := startPostgres(t.Context(), d, "x")
			if err == nil || !strings.Contains(err.Error(), tc.want) || pg.name != "monaco-verify-x" {
				t.Fatalf("startPostgres = %v, want %q with the container named for removal", err, tc.want)
			}
		})
	}
}

func TestRemove_treatsAMissingContainerAsRemoved(t *testing.T) {
	t.Parallel()
	gone, _ := fakeDocker(t, "exit 0", `echo "Error: No such container: x" >&2; exit 1`, "exit 0")
	if err := (&postgres{docker: gone, name: "x"}).remove(t.Context()); err != nil {
		t.Fatalf("remove of a missing container = %v", err)
	}
	stuck, _ := fakeDocker(t, "exit 0", `echo "daemon busy" >&2; exit 1`, "exit 0")
	if err := (&postgres{docker: stuck, name: "x"}).remove(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "daemon busy") {
		t.Fatalf("remove = %v, want the docker error", err)
	}
}

func TestEnsureImage_pullsOnlyWhenTheImageIsMissing(t *testing.T) {
	t.Parallel()
	present, calls := fakeDocker(t, "exit 0", "exit 0", "exit 0")
	if err := present.ensureImage(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := readCalls(t, calls); strings.Contains(got, "pull") {
		t.Fatalf("pulled a present image: %s", got)
	}
	missing, _ := fakeDocker(t, "exit 0", "exit 0", "exit 1")
	if err := missing.ensureImage(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "pull failed") {
		t.Fatalf("ensureImage = %v, want the pull failure", err)
	}
}
