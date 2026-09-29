package verify

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestSelectUnits_picksOutcomesByTargetAndSkipsPlannedFlows(t *testing.T) {
	t.Parallel()
	all, err := readFlows(backendDir(t))
	if err != nil {
		t.Fatal(err)
	}
	all = append(all, tools.Flow{ID: "98", Status: tools.StatusPlanned, Outcomes: []tools.Outcome{"ok"}})
	for _, tc := range []struct {
		target Target
		want   []string
	}{
		{Target{}, []string{"00 ok", "00 InvalidInput", "00 Unauthorized"}},
		{Target{Flow: "00", Outcome: "Unauthorized"}, []string{"00 Unauthorized"}},
		{Target{CrashAt: "after-publish"}, []string{"00 crash:after-publish"}},
	} {
		units, err := selectUnits(all, tc.target, flows.Scripts())
		got := make([]string, 0, len(units))
		for _, u := range units {
			got = append(got, u.Name())
		}
		if err != nil || strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("selectUnits(%+v) = %v, %v, want %v", tc.target, got, err, tc.want)
		}
	}
	if _, err := selectUnits(all, Target{Flow: "98"}, flows.Scripts()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("selectUnits of a planned flow = %v, want nothing to verify", err)
	}
}

func TestParseArgs_aCrashOutcomeArmsItsFaultpoint(t *testing.T) {
	t.Parallel()
	got, err := ParseArgs([]string{"flow", "00", "--outcome", "crash:after-publish"})
	if err != nil || got.CrashAt != "after-publish" {
		t.Fatalf("ParseArgs = %+v, %v, want --crash-at after-publish implied", got, err)
	}
}

func TestReadFlows_rejectsAMalformedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tools.File), []byte("id\tflow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFlows(dir); !errors.Is(err, fs.ErrInvalid) || !strings.Contains(err.Error(), "header must be") {
		t.Fatalf("readFlows = %v, want the header problem", err)
	}
}

func servedDriver(t *testing.T) (*driver, *Result) {
	t.Helper()
	d, err := newDriver(servedEnv(t), DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	res := d.run(t.Context(), flow00(t, Target{Flow: "00", Outcome: "ok"})[0])
	if !res.Pass() || len(res.Events) != 1 {
		t.Fatalf("flow 00 ok = %+v", res)
	}
	return d, res
}

func TestDriver_reportsDatabaseFailures(t *testing.T) {
	t.Parallel()
	d, res := servedDriver(t)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.flowEvents(cancelled, res.Users); err == nil {
		t.Error("flowEvents on a cancelled context succeeded")
	}
	if _, err := d.stuck(cancelled, nil, res.Events); err == nil {
		t.Error("stuck on a cancelled context succeeded")
	}
	closed, err := pgxpool.New(t.Context(), d.env.Pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	d.env.Pool = closed
	if err := d.settle(t.Context(), &Result{Unit: res.Unit}); err == nil {
		t.Error("settle on a closed pool succeeded")
	}
}

func TestDriver_failsWithoutATokenKey(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	cfg, _ := driveConfig(DefaultBudget())
	env.TokenKey = ""
	if err := verifyUnits(t.Context(), cfg, env, nil, 1); err == nil {
		t.Error("verifyUnits without a token key succeeded")
	}
	(&flowT{}).Helper()
}
