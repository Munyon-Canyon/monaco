package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type alertQuery struct {
	RefID string `json:"refId"`
	Model struct {
		Expr string `json:"expr"`
	} `json:"model"`
}

type alertRule struct {
	UID         string            `json:"uid"`
	Title       string            `json:"title"`
	Condition   string            `json:"condition"`
	For         string            `json:"for"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Data        []alertQuery      `json:"data"`
}

type alertGroup struct {
	FolderUID string      `json:"folderUID"`
	RuleGroup string      `json:"ruleGroup"`
	Interval  string      `json:"interval"`
	Rules     []alertRule `json:"rules"`
}

func launchDayGroup(t *testing.T) alertGroup {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..")
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	raw, err := repo.ReadFile("ops/grafana/alerts/launch-day.json")
	if err != nil {
		t.Fatal(err)
	}
	var group alertGroup
	if err := json.Unmarshal(raw, &group); err != nil {
		t.Fatal(err)
	}
	return group
}

func (r alertRule) expression() string {
	var b strings.Builder
	for _, q := range r.Data {
		b.WriteString(q.Model.Expr)
	}
	return b.String()
}

func (r alertRule) problem() string {
	switch {
	case r.UID == "" || r.Title == "" || r.For == "" || r.Condition == "":
		return "an empty uid, title, for or condition"
	case r.Labels["team"] == "" || r.Labels["severity"] == "":
		return "no team or severity label"
	case r.Annotations["summary"] == "":
		return "no summary"
	case r.expression() == "":
		return "no query expression"
	}
	return ""
}

func TestAlertRules_everyRuleNamesAnExpressionAnOwnerAndAHandlingNote(t *testing.T) {
	t.Parallel()
	group := launchDayGroup(t)
	if group.FolderUID == "" || group.RuleGroup == "" || group.Interval == "" || len(group.Rules) == 0 {
		t.Fatalf("group = %+v, want a folder, a group name, an interval and rules", group)
	}
	seen := map[string]bool{}
	for _, r := range group.Rules {
		if p := r.problem(); p != "" {
			t.Errorf("rule %q has %s", r.UID, p)
		}
		if seen[r.UID] {
			t.Errorf("rule uid %q appears twice", r.UID)
		}
		seen[r.UID] = true
	}
}

func TestAlertRules_coverTheLaunchDaySignals(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"monaco-money-poller-stalled":     "poller_last_success_timestamp_seconds",
		"monaco-events-stream-storage-70": "monaco_bus_account_storage_bytes",
		"monaco-money-intent-oldest":      "monaco_money_intent_oldest_seconds",
		"monaco-outbox-relay-lag":         "monaco_events_oldest_unpublished_seconds",
		"monaco-consumer-backlog":         "monaco_bus_consumer_pending",
		"monaco-dead-letters-open":        "monaco_dead_letters",
		"monaco-telemetry-absent":         "absent(",
	}
	exprs := map[string]string{}
	for _, r := range launchDayGroup(t).Rules {
		exprs[r.UID] = r.expression()
	}
	for uid, needle := range want {
		if !strings.Contains(exprs[uid], needle) {
			t.Errorf("rule %s expression %q lacks %q", uid, exprs[uid], needle)
		}
	}
}
