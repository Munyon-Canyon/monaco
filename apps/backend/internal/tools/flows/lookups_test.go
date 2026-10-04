package flows_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const spec = `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /v1/cabals/{id}/fund:
    post:
      responses: {"204": {description: ok}}
  /healthz:
    get:
      responses: {"200": {description: ok}}
`

func TestRoutes_listsEveryMethodAndPathInTheSpecSorted(t *testing.T) {
	t.Parallel()
	want := []string{"GET /healthz", "POST /v1/cabals/{id}/fund"}
	if got := flows.Routes([]byte(spec)); !slices.Equal(got, want) {
		t.Fatalf("Routes = %q, want %q", got, want)
	}
	if got := flows.Routes([]byte("openapi: [unclosed")); got != nil {
		t.Fatalf("Routes of an unparsable spec = %q, want none", got)
	}
}

func TestTriggers_acceptsARouteAConsumedSubjectARegisteredPollerOrAnOperation(t *testing.T) {
	t.Parallel()
	lookup := flows.Triggers([]byte(spec), []string{"proposal.passed"}, []string{"deposits"})
	for trigger, want := range map[string]bool{
		"POST /v1/cabals/{id}/fund": true,
		"GET /v1/cabals/{id}/fund":  false,
		"consumer:proposal.passed":  true,
		"consumer:proposal.failed":  false,
		"poller:deposits":           true,
		"poller:prices":             false,
		"poller:proposal.passed":    false,
		"consumer:deposits":         false,
		"deposits":                  false,
	} {
		if got := lookup(flows.Flow{}, trigger); got != want {
			t.Errorf("trigger %q = %v, want %v", trigger, got, want)
		}
	}
	if !lookup(flows.Flow{Commands: []string{"VoidProposal"}}, "ops:VoidProposal") {
		t.Error("ops trigger did not match its flow command")
	}
	if lookup(flows.Flow{Commands: []string{"VoidProposal"}}, "ops:Other") {
		t.Error("ops trigger matched an unrelated command")
	}
}

func TestTriggerKind_splitsThePollerAndConsumerPrefixesAndTreatsTheRestAsARoute(t *testing.T) {
	t.Parallel()
	for trigger, want := range map[string]struct {
		kind flows.TriggerKind
		name string
	}{
		"POST /v1/system/pings":    {flows.TriggerRoute, "POST /v1/system/pings"},
		"poller:market.prices":     {flows.TriggerPoller, "market.prices"},
		"consumer:proposal.passed": {flows.TriggerConsumer, "proposal.passed"},
		"ops:VoidProposal":         {flows.TriggerOps, "VoidProposal"},
		"poller":                   {flows.TriggerRoute, "poller"},
		"deposits:poller":          {flows.TriggerRoute, "deposits:poller"},
	} {
		if kind, name := (flows.Flow{Trigger: trigger}).TriggerKind(""); kind != want.kind || name != want.name {
			t.Errorf("TriggerKind(%q) = %s %q, want %s %q", trigger, kind, name, want.kind, want.name)
		}
	}
}

func TestMembers_matchesOnlyListedValues(t *testing.T) {
	t.Parallel()
	lookup := flows.Members([]string{"treasury.positions", "ranking"})
	if !lookup(flows.Flow{}, "ranking") || lookup(flows.Flow{}, "feed") {
		t.Fatal("Members did not match exactly the listed values")
	}
}

func TestCommands_findsOnlyTypesDeclaredInTheFlowsModuleAppPackage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":                                "module example.com/m\n\ngo 1.25\n",
		"internal/modules/system/app/app.go":    "package app\n\ntype Ping struct{}\n\nfunc Helper() {}\n",
		"internal/modules/broken/app/broken.go": "package app\n\ntype Ping struct{ missing Undefined }\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lookup := flows.Commands(dir)
	for _, tc := range []struct {
		module, command string
		want            bool
	}{
		{"system", "Ping", true},
		{"system", "Ping", true},
		{"system", "Helper", false},
		{"system", "Pong", false},
		{"ghost", "Ping", false},
		{"broken", "Ping", false},
	} {
		if got := lookup(flows.Flow{Module: tc.module}, tc.command); got != tc.want {
			t.Errorf("command %s in module %s = %v, want %v", tc.command, tc.module, got, tc.want)
		}
	}
	if flows.Commands(filepath.Join(dir, "missing"))(flows.Flow{Module: "system"}, "Ping") {
		t.Error("a command resolved from a backend directory that does not exist")
	}
}

func TestTriggerKind_picksEachCommandsTrigger(t *testing.T) {
	t.Parallel()
	perCommand := flows.Flow{
		Trigger:  "POST /v1/cabals/{id}/members; poller:cabal.sweep",
		Commands: []string{"JoinCabal", "Sweep"},
	}
	shared := flows.Flow{Trigger: "POST /v1/cabals/{id}/members", Commands: []string{"JoinCabal", "RequestAccess"}}
	for _, tc := range []struct {
		flow    flows.Flow
		command string
		kind    flows.TriggerKind
		name    string
	}{
		{perCommand, "JoinCabal", flows.TriggerRoute, "POST /v1/cabals/{id}/members"},
		{perCommand, "Sweep", flows.TriggerPoller, "cabal.sweep"},
		{perCommand, "Unknown", flows.TriggerRoute, "POST /v1/cabals/{id}/members"},
		{shared, "RequestAccess", flows.TriggerRoute, "POST /v1/cabals/{id}/members"},
		{flows.Flow{}, "", flows.TriggerRoute, ""},
	} {
		if kind, name := tc.flow.TriggerKind(tc.command); kind != tc.kind || name != tc.name {
			t.Errorf("TriggerKind(%q) = %s %q, want %s %q", tc.command, kind, name, tc.kind, tc.name)
		}
	}
}
