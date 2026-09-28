package lint_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB(), testkit.WithSetup(buildTools))
}

var (
	backendRoot string
	genDepguard string
	nogoBin     string
)

func buildTools() (func(), error) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		return nil, fmt.Errorf("backend root: %w", err)
	}
	bin, err := os.MkdirTemp("", "lint-rules-")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(bin) }
	backendRoot = root
	genDepguard = filepath.Join(bin, "gen-depguard")
	nogoBin = filepath.Join(bin, "nogo")
	for out, pkg := range map[string]string{
		genDepguard: "./scripts/gen-depguard",
		nogoBin:     "./internal/platform/lint/nogo/cmd/nogo",
	} {
		build := exec.CommandContext(context.Background(), "go", "build", "-o", out, pkg)
		build.Dir = root
		if b, err := build.CombinedOutput(); err != nil {
			cleanup()
			return nil, fmt.Errorf("go build %s: %w\n%s", pkg, err, b)
		}
	}
	return cleanup, nil
}
