package replay_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

func TestCheckTarget_refusesTheSourceAndTheDevDatabase(t *testing.T) {
	t.Parallel()
	const source = "postgres://monaco@localhost:54323/monaco"
	for target, want := range map[string]string{
		source: "target is the source database",
		"postgres://other@127.0.0.1:54323/monaco?sslmode=disable": "target is the source database",
		"postgres://monaco@localhost:54322/replay":                "target is on the dev database container",
		"postgres://monaco@localhost:54323/replay":                "",
		"postgres://monaco@db.example:54323/monaco":               "",
		"postgres://%zz": "cannot parse",
	} {
		err := replay.CheckTarget(source, target)
		if want == "" && err == nil {
			continue
		}
		if err == nil || errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckTarget(%s) = %v, want %q", target, err, want)
		}
	}
}
