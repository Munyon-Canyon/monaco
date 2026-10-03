package system_test

import (
	"bytes"
	"net/http"
	"testing"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestRecordPing_overTheActorRateLimitIsRefusedWithoutAPing(t *testing.T) {
	t.Parallel()
	const anchor = "      operationId: postSystemPing\n"
	spec := bytes.Replace(openapi.Spec, []byte(anchor),
		[]byte(anchor+"      x-rate-limit: {actor: {rate: 1, per: 1m, burst: 1}}\n"), 1)
	if bytes.Equal(spec, openapi.Spec) {
		t.Fatalf("anchor %q is not in api/openapi.yaml", anchor)
	}
	limited, err := scenario.LoadContract(spec)
	if err != nil {
		t.Fatal(err)
	}
	scenario.New(t, withSystem(), scenario.WithContract(limited)).
		Given(scenario.AsUser("alice")).
		When(
			scenario.Post("/v1/system/pings", `{"note":"one"}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Post("/v1/system/pings", `{"note":"two"}`),
		).
		Then(
			scenario.ExpectStatus(http.StatusTooManyRequests),
			scenario.ExpectProblem(errs.CodeRateLimited),
			scenario.ExpectEvents(events.TypeSystemPinged, 1),
			scenario.AsUser("bob"),
			scenario.Post("/v1/system/pings", `{"note":"three"}`),
			scenario.ExpectStatus(http.StatusCreated),
		)
}
