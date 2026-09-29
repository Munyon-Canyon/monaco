package verify

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

func build(ctx context.Context, goBin, dir, out string, faultpoints bool) (Binaries, error) {
	args := []string{"build", "-cover"}
	if faultpoints {
		args = append(args, "-tags", "faultpoints")
	}
	args = append(args, "-o", out+string(filepath.Separator), "./cmd/api", "./cmd/worker", "./cmd/fakes")
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return Binaries{}, fmt.Errorf("go build: %w\n%s", err, output)
	}
	return Binaries{
		API:    filepath.Join(out, "api"),
		Worker: filepath.Join(out, "worker"),
		Fakes:  filepath.Join(out, "fakes"),
	}, nil
}
