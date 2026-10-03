package flows_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const baseSpec = `openapi: 3.1.0
info: {title: Monaco, version: 0.1.0}
security: [{bearerAuth: []}]
paths:
  /v1/auth/session:
    post:
      responses:
        "200": {$ref: "#/components/responses/Session"}
  /v1/cabals:
    post:
      requestBody:
        content:
          application/json:
            schema: {$ref: "#/components/schemas/NewCabal"}
      responses:
        "201":
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Cabal"}
  /v1/cabals/{id}:
    parameters: [{$ref: "#/components/parameters/ID"}]
    get:
      responses:
        "200":
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Cabal"}
  /v1/me/handle:
    put:
      responses:
        "204": {description: Set.}
components:
  parameters:
    ID: {name: id, in: path, required: true, schema: {type: string}}
  responses:
    Session:
      description: A session.
      content:
        application/json:
          schema: {$ref: "#/components/schemas/User"}
  schemas:
    NewCabal: {type: object, properties: {name: {type: string}}}
    Cabal: {type: object, properties: {name: {type: string}, owner: {$ref: "#/components/schemas/User"}}}
    User: {type: object, properties: {handle: {type: string}}}
`

func edit(from, to string) []byte { return []byte(strings.Replace(baseSpec, from, to, 1)) }

func TestChangedOperations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		head []byte
		want []string
	}{
		{"nothing", []byte(baseSpec), nil},
		{"a description outside paths", edit("title: Monaco", "title: Monaco API"), nil},
		{
			"a new route",
			edit("  /v1/me/handle:", "  /v1/users/{id}/follow:\n    post:\n      responses:\n"+
				"        \"204\": {description: Followed.}\n  /v1/me/handle:"),
			[]string{"POST /v1/users/{id}/follow"},
		},
		{"one route's own body", edit("description: Set.", "description: Handle set."), []string{"PUT /v1/me/handle"}},
		{"a schema one route uses", edit("NewCabal: {type: object, properties: {name: {type: string}}}",
			"NewCabal: {type: object, properties: {name: {type: integer}}}"), []string{"POST /v1/cabals"}},
		{
			"a shared schema", edit("Cabal: {type: object, properties: {name: {type: string},",
				"Cabal: {type: object, properties: {name: {type: integer},"),
			[]string{"GET /v1/cabals/{id}", "POST /v1/cabals"},
		},
		{
			"a schema reached through refs", edit("User: {type: object, properties: {handle: {type: string}}}",
				"User: {type: object, properties: {handle: {type: integer}}}"),
			[]string{"GET /v1/cabals/{id}", "POST /v1/auth/session", "POST /v1/cabals"},
		},
		{"a path parameter", edit("ID: {name: id, in: path, required: true, schema: {type: string}}",
			"ID: {name: id, in: path, required: true, schema: {type: integer}}"), []string{"GET /v1/cabals/{id}"}},
		{
			"global security", edit("security: [{bearerAuth: []}]", "security: []"),
			[]string{"GET /v1/cabals/{id}", "POST /v1/auth/session", "POST /v1/cabals", "PUT /v1/me/handle"},
		},
		{
			"an unreadable head", []byte("paths: ["),
			[]string{"GET /v1/cabals/{id}", "POST /v1/auth/session", "POST /v1/cabals", "PUT /v1/me/handle"},
		},
	} {
		if got := flows.ChangedOperations([]byte(baseSpec), tc.head); !slices.Equal(got, tc.want) {
			t.Errorf("%s: ChangedOperations = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAffected_specChangesReachOnlyTheFlowsWhoseRoutesChanged(t *testing.T) {
	t.Parallel()
	registry := []flows.Flow{
		{ID: "01", Trigger: "POST /v1/auth/session"},
		{ID: "01a", Trigger: "PUT /v1/me/handle"},
		{ID: "02", Trigger: "POST /v1/cabals"},
		{ID: "20", Trigger: "POST /v1/users/{id}/follow"},
		{ID: "18", Trigger: "poller:market.prices"},
	}
	changed := []string{flows.SpecPath, "apps/backend/api/spec/social.yaml"}
	for _, tc := range []struct {
		name string
		head []byte
		want []string
	}{
		{"a stack that adds one route", edit("  /v1/me/handle:", "  /v1/users/{id}/follow:\n    post:\n"+
			"      responses:\n        \"204\": {description: Followed.}\n  /v1/me/handle:"), []string{"20"}},
		{"a shared schema", edit("User: {type: object, properties: {handle: {type: string}}}",
			"User: {type: object, properties: {handle: {type: integer}}}"), []string{"01", "02"}},
	} {
		ops := flows.ChangedOperations([]byte(baseSpec), tc.head)
		if got := flows.Affected(changed, ops, registry); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Affected = %q, want %q", tc.name, got, tc.want)
		}
	}
}
