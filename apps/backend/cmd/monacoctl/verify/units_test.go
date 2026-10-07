package verify

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestSelectUnits_picksOutcomesByTargetAndSkipsPlannedFlows(t *testing.T) {
	t.Parallel()
	all := []tools.Flow{
		{
			ID: "10", Status: tools.StatusBuilt, Commands: []string{"CastVote"},
			Outcomes: []tools.Outcome{"ok", "NotAVoter", "crash:after-publish"},
		},
		{
			ID: "20", Status: tools.StatusBuilt, Commands: []string{"Follow", "Unfollow"},
			Outcomes: []tools.Outcome{"ok", "Unauthorized", "crash:before-commit"},
		},
		{ID: "98", Status: tools.StatusPlanned, Commands: []string{"Plan"}, Outcomes: []tools.Outcome{"ok"}},
	}
	noop := func(*scenario.Scenario) {}
	scripts := map[string]flows.Script{}
	for _, f := range all {
		for _, command := range f.Commands {
			for _, o := range f.Outcomes {
				scripts[tools.ScriptName(f, command, o)] = noop
			}
		}
	}
	delete(scripts, tools.ScriptName(all[1], "Unfollow", "Unauthorized"))
	delete(scripts, tools.ScriptName(all[1], "Unfollow", "crash:before-commit"))
	for _, tc := range []struct {
		target Target
		want   []string
	}{
		{Target{}, []string{"10 ok", "10 NotAVoter", "20 Follow ok", "20 Unfollow ok", "20 Follow Unauthorized"}},
		{Target{Flow: "10", Outcome: "NotAVoter"}, []string{"10 NotAVoter"}},
		{Target{CrashAt: "after-publish"}, []string{"10 crash:after-publish"}},
		{Target{CrashAt: "before-commit"}, []string{"20 Follow crash:before-commit"}},
	} {
		units, err := selectUnits(all, tc.target, scripts)
		got := make([]string, 0, len(units))
		for _, u := range units {
			got = append(got, u.Name())
		}
		if err != nil || strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("selectUnits(%+v) = %v, %v, want %v", tc.target, got, err, tc.want)
		}
	}
	if _, err := selectUnits(all, Target{Flow: "98"}, scripts); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("selectUnits of a planned flow = %v, want nothing to verify", err)
	}
	unscripted := tools.Flow{
		ID: "30", Status: tools.StatusBuilt, Commands: []string{"Mute"}, Outcomes: []tools.Outcome{"ok"},
	}
	missing := tools.ScriptName(unscripted, "Mute", "ok")
	_, err := selectUnits(append(all, unscripted), Target{}, scripts)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), missing) {
		t.Fatalf("selectUnits of an outcome with no script = %v, want an error naming %s", err, missing)
	}
}

func TestParseArgs_aCrashOutcomeArmsItsFaultpoint(t *testing.T) {
	t.Parallel()
	got, err := ParseArgs([]string{"flow", "00", "--outcome", "crash:after-publish"})
	if err != nil || got.CrashAt != "after-publish" {
		t.Fatalf("ParseArgs = %+v, %v, want --crash-at after-publish implied", got, err)
	}
}

func TestFlow09Scripts_pinEveryProposeTradeOutcome(t *testing.T) {
	t.Parallel()
	flow := tools.Flow{
		ID: "09", Status: tools.StatusBuilt, Commands: []string{"ProposeTrade"},
		Outcomes: []tools.Outcome{
			"ok", "InvalidInput", "Unauthorized", "NotCabalMember", "AssetNotFound", "AssetUntradable",
			"NoRoute", "PotExceeded", "InsufficientFunds", "JupiterUnavailable", "PriceUnavailable",
			"crash:after-publish",
		},
	}
	for _, outcome := range flow.Outcomes {
		name := tools.ScriptName(flow, flow.Commands[0], outcome)
		if _, ok := flows.Scripts()[name]; !ok {
			t.Errorf("Scripts() has no %s", name)
		}
	}
}

func TestReadFlows_rejectsAMalformedFile(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "apps", "backend")
	writeFlows(t, dir, "id\tflow\n90\tHealth\n")
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
	d, err := newDriver(servedEnv(t), testBudget())
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
	if _, err := d.flowEvents(cancelled, res.Users, nil); err == nil {
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
	d.env.Arm = func(context.Context, Unit) error { return errors.New("worker would not restart") }
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

func TestSelectUnits_runsEachCommandsScriptForAnOutcomeOfAMultiCommandFlow(t *testing.T) {
	t.Parallel()
	f := tools.Flow{
		ID: "97", Status: tools.StatusBuilt, Commands: []string{"Join", "Leave"},
		Outcomes: []tools.Outcome{"ok", "Blocked"},
	}
	noop := func(*scenario.Scenario) {}
	scripts := map[string]flows.Script{"F97JoinOK": noop, "F97LeaveOK": noop, "F97LeaveBlocked": noop}
	units, err := selectUnits([]tools.Flow{f}, Target{}, scripts)
	got := make([]string, 0, len(units))
	for _, u := range units {
		got = append(got, u.Name())
	}
	if want := "97 Join ok,97 Leave ok,97 Leave Blocked"; err != nil || strings.Join(got, ",") != want {
		t.Fatalf("selectUnits = %v, %v, want %s", got, err, want)
	}
	delete(scripts, "F97LeaveBlocked")
	_, err = selectUnits([]tools.Flow{f}, Target{}, scripts)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "F97JoinBlocked or F97LeaveBlocked") {
		t.Fatalf("selectUnits without a Blocked script = %v, want both candidate names", err)
	}
}
