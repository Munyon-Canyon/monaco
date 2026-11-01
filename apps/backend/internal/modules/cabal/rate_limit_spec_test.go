package cabal_test

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
)

type actorLimit struct {
	Actor struct {
		Rate  int    `json:"rate"`
		Per   string `json:"per"`
		Burst int    `json:"burst"`
	} `json:"actor"`
}

func actorLimits(t *testing.T) map[string]actorLimit {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	limits := map[string]actorLimit{}
	for _, item := range doc.Paths.Map() {
		for _, operation := range item.Operations() {
			raw, ok := operation.Extensions[ratelimit.Extension]
			if !ok {
				continue
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var limit actorLimit
			if err := json.Unmarshal(encoded, &limit); err != nil {
				t.Fatal(err)
			}
			limits[operation.OperationID] = limit
		}
	}
	return limits
}

func TestSpec_everyJoinRequestRevokeAndLeaveRouteIsLimitedPerActorTwentyAnHour(t *testing.T) {
	t.Parallel()
	limits := actorLimits(t)
	for _, operationID := range []string{
		"postCabalMember", "deleteCabalMemberMe", "postCabalAccessRequest", "deleteCabalAccessRequest",
		"postCabalInvite",
	} {
		got := limits[operationID].Actor
		if got.Rate != 20 || got.Per != "1h" || got.Burst != 20 {
			t.Errorf("%s limit = %+v, want 20 an hour with a burst of 20", operationID, got)
		}
	}
}
