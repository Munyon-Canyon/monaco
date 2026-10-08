package social_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withSocialAndAdmin() scenario.Option {
	return scenario.WithModules(
		func(d module.Deps) module.Module { return social.New(d) },
		func(d module.Deps) module.Module { return admin.New(d) },
	)
}

func reportUser(reporter, target string) []scenario.Step {
	return []scenario.Step{
		scenario.AsUser(reporter),
		func(s *scenario.Scenario) {
			s.Helper()
			scenario.Post("/v1/reports", `{"kind":"user","target_id":"`+s.Recall(target)+`","reason":"spam"}`)(s)
		},
		scenario.ExpectStatus(http.StatusCreated),
	}
}

func reportersAre(want ...string) scenario.Step {
	return scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
		s.Helper()
		var items []struct {
			Reporter struct {
				UserID string `json:"user_id"`
			} `json:"reporter"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &items); err != nil || len(items) != len(want) {
			s.Fatalf("reports = %s (%v), want %d of them", raw, err, len(want))
		}
		for i, name := range want {
			if items[i].Reporter.UserID != s.Recall(name) || items[i].Status != "open" {
				s.Fatalf("report %d = %+v, want an open report by %s", i, items[i], name)
			}
		}
	})
}

func TestAdminReports_Moderator(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withSocialAndAdmin())
	s.Given(
		scenario.SeededUser("alice", "active"), scenario.SeededUser("bob", "active"),
		scenario.SeededUser("cara", "active"), scenario.SeededAdmin("mod", "moderator"),
		scenario.SeededAdmin("viewer", "viewer"),
	)
	steps := append(reportUser("alice", "bob"), reportUser("cara", "bob")...)
	steps = append(steps, reportUser("bob", "alice")...)
	s.When(append(steps,
		scenario.AsUser("mod"),
		scenario.Get("/v1/admin/reports?limit=2"),
		scenario.ExpectStatus(http.StatusOK),
		reportersAre("alice", "cara"),
		scenario.Remember("next_cursor", "cursor"),
		scenario.Get("/v1/admin/reports?limit=2&cursor={cursor}"),
		scenario.ExpectStatus(http.StatusOK),
		reportersAre("bob"),
		scenario.ExpectJSON("next_cursor", nil),
	)...).Then(
		scenario.AsUser("viewer"),
		scenario.Get("/v1/admin/reports"),
		scenario.ExpectProblem(errs.CodeAdminForbidden),
		scenario.AsUser("alice"),
		scenario.Get("/v1/admin/reports"),
		scenario.ExpectProblem(errs.CodeAdminForbidden),
	)
}
