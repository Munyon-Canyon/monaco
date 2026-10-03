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
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
		{Target{}, []string{
			"00 ok", "00 InvalidInput", "00 Unauthorized", "01 ok", "01 Unauthorized", "01 LoginMethodNotAllowed",
			"01 AccountDeleted", "01 PrivyUnavailable", "01a ok", "01a HandleInvalid", "01a HandleReserved",
			"01a HandleTaken", "01a HandleTooSoon", "02 ok", "02 InvalidInput", "02 Unauthorized",
			"02 PrivyUnavailable", "10 ok", "10 Unauthorized", "10 ProposalNotFound", "10 NotAVoter",
			"10 ProposalClosed", "18 ok", "18 JupiterUnavailable", "18 UpstreamTimeout", "20 ok",
			"20 CannotFollowSelf", "20 UserNotFound", "20 UserBanned", "20 Unauthorized", "23 ok",
			"23 DisplayNameInvalid", "23a ok", "23a PhotoInvalid", "23a StorageUnavailable", "23a RateLimited",
		}},
		{Target{Flow: "00", Outcome: "Unauthorized"}, []string{"00 Unauthorized"}},
		{Target{CrashAt: "after-publish"}, []string{"00 crash:after-publish", "10 crash:after-publish"}},
		{Target{CrashAt: "before-commit"}, []string{
			"01 crash:before-commit", "02 crash:before-commit", "20 crash:before-commit",
		}},
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

func TestInvariantHelpers(t *testing.T) {
	t.Parallel()
	if pathMatches("/v1/x/{id}", "/v1/x/1/y") || !pathMatches("/v1/x/{id}", "/v1/x/1") ||
		pathMatches("/v1/x", "/v1/y") {
		t.Error("pathMatches")
	}
	if codeNamed("NoSuchCode") != "NoSuchCode" || codeNamed("InvalidInput") != "invalid_input" {
		t.Error("codeNamed")
	}
	if len(LedgerChecks()) != 0 {
		t.Error("LedgerChecks has entries; give each a test")
	}
	if got := (&InvariantError{Msg: "x"}).Error(); got != "invariant: x" {
		t.Errorf("InvariantError = %q", got)
	}
	(&flowT{}).Helper()
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

func TestDriver_ignoresOtherTypesAndNamesAnIdleConsumer(t *testing.T) {
	t.Parallel()
	d, res := servedDriver(t)
	other := []watched{{durable: "x", handler: "x", typ: "other"}}
	if stuck, err := d.stuck(t.Context(), other, res.Events); stuck != "" || err != nil {
		t.Errorf("stuck with a handler of another type = %q, %v", stuck, err)
	}
	if _, err := d.env.JS.CreateConsumer(t.Context(), d.env.Events, jetstream.ConsumerConfig{
		Durable: "idle", AckPolicy: jetstream.AckExplicitPolicy,
	}); err != nil {
		t.Fatal(err)
	}
	idle := []watched{{durable: "idle", handler: "system.echo", typ: "system.pinged"}}
	if stuck, err := d.stuck(t.Context(), idle, res.Events); !strings.Contains(stuck, "consumer idle has 1 pending") ||
		err != nil {
		t.Errorf("stuck with an idle consumer = %q, %v", stuck, err)
	}
}

func TestDriver_reportsNATSArmAndTokenFailures(t *testing.T) {
	t.Parallel()
	env := unservedEnv("")
	env.JS = testkit.NATS(t).JS
	d, err := newDriver(env, DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	d.env.DeadLetter = "MISSING"
	if got := d.global(
		t.Context(),
		nil,
		&report{},
	); len(got) != 1 ||
		!strings.Contains(got[0].Error(), "read MISSING") {
		t.Errorf("global without the dead letter stream = %v", got)
	}
	d.env.Arm = func(context.Context) error { return errors.New("worker would not restart") }
	u := flow00(t, Target{Flow: "00", Outcome: "ok"})[0]
	if res := d.run(t.Context(), u); res.Failure != "worker would not restart" {
		t.Errorf("run with a failing arm = %+v", res)
	}
	cfg, _ := driveConfig(DefaultBudget())
	env.TokenKey = ""
	if err := verifyUnits(t.Context(), cfg, env, &report{}, 1); err == nil {
		t.Error("verifyUnits without a token key succeeded")
	}
}
