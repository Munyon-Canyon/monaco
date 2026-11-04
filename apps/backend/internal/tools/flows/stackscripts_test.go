package flows_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const directMomentScripts = `package flows

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type momentWriter struct {
	handler *app.WriteChatMomentHandler
}

func newMomentWriter(s *scenario.Scenario) momentWriter {
	return momentWriter{handler: app.NewWriteChatMomentHandler(app.ChatDeps{})}
}

func applyFilled(s *scenario.Scenario, w momentWriter, e events.TradeConfirmed) error {
	moments := adapters.ChatMoments{
		Assets: social.NewAssets(nil), Write: w.handler,
	}
	asset, err := moments.FetchFilled(s.Context(), e)
	_ = asset
	return err
}

func F22dWriteTradeFilledMomentOK(s *scenario.Scenario) {
	var w momentWriter
	s.Given(func(s *scenario.Scenario) { w = newMomentWriter(s) }).
		When(func(s *scenario.Scenario) {
			if err := applyFilled(s, w, events.TradeConfirmed{}); err != nil {
				s.Fatalf("flows: apply the fill: %v", err)
			}
		})
}

func F22dWriteMemberJoinedMomentInvalidInput(s *scenario.Scenario) {
	_ = app.NewWriteChatMomentHandler(app.ChatDeps{})
	_, _ = domain.NewMemberJoinedMoment(s.ActorID())
}
`

const busMomentScripts = `package flows

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func publishTradeConfirmed(s *scenario.Scenario) {
	_ = s.Bus()
}

func F22dWriteTradeFilledMomentOK(s *scenario.Scenario) {
	s.Given(chatCabal("moment-fill")...).
		When(publishTradeConfirmed).
		Then(
			scenario.EventuallyHint("moment"),
			scenario.ExpectAblyPublished(app.EventMessageCreated, "cabal", 1),
		)
}

func F22dWriteMemberJoinedMomentInvalidInput(s *scenario.Scenario) {
	s.Given(chatCabal("moment-invalid")...).When(scenario.Get("/v1/x"))
}
`

const consumerTriggers = "consumer:trade.confirmed; consumer:cabal.member_joined"

func stackScriptProblems(src string, status flows.Status) []string {
	return triggeredProblems(src, status, consumerTriggers)
}

func triggeredProblems(src string, status flows.Status, trigger string) []string {
	env := flows.Env{
		BackendDir: "apps/backend",
		Repo: fstest.MapFS{
			"apps/backend/internal/testkit/flows/f22d.go": {Data: []byte(src)},
		},
	}
	f := flows.Flow{
		ID: "22d", Status: status, Trigger: trigger,
		Commands: []string{"WriteTradeFilledMoment", "WriteMemberJoinedMoment"},
		Outcomes: []flows.Outcome{"ok", "InvalidInput"},
	}
	return lines(flows.CheckStackScriptsUseTheBus([]flows.Flow{f}, env))
}

func TestCheckStackScriptsUseTheBus_flagsScriptsThatCallTheModuleDirectly(t *testing.T) {
	t.Parallel()
	got := stackScriptProblems(directMomentScripts, flows.StatusBuilt)
	if len(got) != 2 {
		t.Fatalf("problems = %v, want one per direct script", got)
	}
	for i, want := range []string{
		"f22d.go:16: flow 22d script F22dWriteTradeFilledMomentOK calls app.NewWriteChatMomentHandler in process" +
			" (through newMomentWriter);",
		"f22d.go:39: flow 22d script F22dWriteMemberJoinedMomentInvalidInput calls app.NewWriteChatMomentHandler" +
			" in process;",
	} {
		if !strings.Contains(got[i], want) {
			t.Errorf("problem %d = %q, want it to contain %q", i, got[i], want)
		}
		if !strings.Contains(got[i], "drive the trigger through the bus") {
			t.Errorf("problem %d = %q, want it to name the fix", i, got[i])
		}
	}
}

func TestCheckStackScriptsUseTheBus_flagsTheAdaptersLiteralBehindAHelper(t *testing.T) {
	t.Parallel()
	src := strings.Replace(directMomentScripts, "app.NewWriteChatMomentHandler(app.ChatDeps{})}", "nil}", 1)
	got := stackScriptProblems(src, flows.StatusBuilt)
	if len(got) < 1 || !strings.Contains(got[0], "calls adapters.ChatMoments in process (through applyFilled)") {
		t.Fatalf("problems = %v, want the adapters literal reached through applyFilled", got)
	}
}

func TestCheckStackScriptsUseTheBus_acceptsScriptsThatDriveTheBus(t *testing.T) {
	t.Parallel()
	if got := stackScriptProblems(busMomentScripts, flows.StatusBuilt); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestCheckStackScriptsUseTheBus_skipsFlowsThatAreNotBuilt(t *testing.T) {
	t.Parallel()
	if got := stackScriptProblems(directMomentScripts, flows.StatusPlanned); len(got) != 0 {
		t.Fatalf("problems = %v, want none for a planned flow", got)
	}
}

func TestCheckStackScriptsUseTheBus_leavesRouteFlowsAlone(t *testing.T) {
	t.Parallel()
	got := triggeredProblems(directMomentScripts, flows.StatusBuilt, "POST /v1/a; POST /v1/b")
	if len(got) != 0 {
		t.Fatalf("problems = %v, want none for route triggered commands", got)
	}
}

const aliasedScripts = `package flows

import (
	socialapp "github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func cleanHelper(s *scenario.Scenario) { cleanHelper(s) }

func F22dWriteTradeFilledMomentOK(s *scenario.Scenario) {
	cleanHelper(s)
	cleanHelper(s)
	_ = socialapp.NewWriteChatMomentHandler(socialapp.ChatDeps{})
}
`

func TestCheckStackScriptsUseTheBus_followsImportAliasesAndVisitsEachHelperOnce(t *testing.T) {
	t.Parallel()
	got := stackScriptProblems(aliasedScripts, flows.StatusBuilt)
	if len(got) != 1 || !strings.Contains(got[0], "calls socialapp.NewWriteChatMomentHandler in process;") {
		t.Fatalf("problems = %v, want the aliased constructor", got)
	}
}
