package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileIntegrationPullsThroughPullImage(t *testing.T) {
	b, err := os.ReadFile("mobile-integration.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	if strings.Contains(script, "compose pull") {
		t.Error("mobile-integration.sh pulls through compose, which has no mirror fallback")
	}
	if !strings.Contains(script, `pull-image.sh" "$image"`) {
		t.Error("mobile-integration.sh does not pull the compose images through pull-image.sh")
	}
	if !strings.Contains(script, "up -d --wait --pull never postgres nats") {
		t.Error("mobile-integration.sh lets compose pull on up")
	}
}

func runMobileIntegrationSwift(t *testing.T, workdir string) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	writeExecutable(t, filepath.Join(dir, "bin", "docker"), "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> "+calls+"\n")
	cmd := exec.Command("bash", "mobile-integration-swift.sh", "swift:test", "FIntegrationTests")
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"),
		"MOBILE_INTEGRATION_WORKDIR="+workdir,
		"MI_BINDIR=/bin-dir", "MI_API_URL=http://api", "MI_DATABASE_URL=db", "MI_NATS_URL=nats", "MI_FAKES_URL=http://fakes",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mobile-integration-swift.sh: %v\n%s", err, out)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMobileIntegrationMountsTheCheckoutAtTheWorkdir(t *testing.T) {
	call := runMobileIntegrationSwift(t, "/__w/monaco/monaco")
	for _, want := range []string{":/__w/monaco/monaco ", "-w /__w/monaco/monaco/packages/mobile-core", "MONACO_SEED_DIR=/__w/monaco/monaco/apps/backend"} {
		if !strings.Contains(call, want) {
			t.Errorf("docker run lacks %q:\n%s", want, call)
		}
	}
	if !strings.Contains(call, "swift test --force-resolved-versions -Xswiftc -warnings-as-errors --enable-code-coverage --filter FIntegrationTests") {
		t.Errorf("docker run lacks the flags the cache was built with:\n%s", call)
	}
	if strings.Contains(call, ":/w ") {
		t.Errorf("docker run still mounts /w:\n%s", call)
	}
}

func TestMobileIntegrationWorkdirDefaultsToW(t *testing.T) {
	call := runMobileIntegrationSwift(t, "")
	for _, want := range []string{":/w ", "-w /w/packages/mobile-core", "MONACO_SEED_DIR=/w/apps/backend"} {
		if !strings.Contains(call, want) {
			t.Errorf("docker run lacks %q:\n%s", want, call)
		}
	}
}
