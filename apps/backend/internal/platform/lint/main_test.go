package lint_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	backendRoot string
	genDepguard string
	nogoBin     string
)

func TestMain(m *testing.M) {
	code, err := runWithTools(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runWithTools(m *testing.M) (int, error) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		return 0, fmt.Errorf("backend root: %w", err)
	}
	bin, err := os.MkdirTemp("", "lint-rules-")
	if err != nil {
		return 0, fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(bin) }()
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
			return 0, fmt.Errorf("go build %s: %w\n%s", pkg, err, b)
		}
	}
	return m.Run(), nil
}
