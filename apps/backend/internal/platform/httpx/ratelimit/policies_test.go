package ratelimit_test

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
)

func spec(topSecurity, paths string) []byte {
	return []byte(`openapi: 3.1.0
info: {title: t, version: "1"}
security: ` + topSecurity + `
components:
  securitySchemes:
    bearerAuth: {type: http, scheme: bearer}
paths:
` + paths)
}

func thing(method, security, extension string) string {
	out := "  /v1/things:\n    " + method + ":\n      operationId: " + method + "Thing\n"
	if security != "" {
		out += "      security: " + security + "\n"
	}
	if extension != "" {
		out += "      x-rate-limit: " + extension + "\n"
	}
	return out + "      responses: {\"204\": {description: done}}\n"
}

const authed = "[{bearerAuth: []}]"

func TestLoad_TheAPISpecLoads(t *testing.T) {
	t.Parallel()
	if _, err := ratelimit.Load(openapi.Spec); err != nil {
		t.Fatalf("Load(api/openapi.yaml) = %v", err)
	}
}

func TestLoad_APublicMutatingRouteWithoutARateLimitFailsTheSpec(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		doc       []byte
		operation string
	}{
		"operation opts out of auth": {spec(authed, thing("post", "[]", "")), "postThing"},
		"spec has no auth":           {spec("[]", thing("delete", "", "")), "deleteThing"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ratelimit.Load(tc.doc)
			if errs.CodeOf(err) != errs.CodeInvalidConfig {
				t.Fatalf("Load = %v, want invalid_config", err)
			}
			if named := slog.String("operation", tc.operation); !slices.ContainsFunc(errs.Detail(err), named.Equal) {
				t.Fatalf("detail %v does not name %s", errs.Detail(err), tc.operation)
			}
		})
	}
}

func TestLoad_RoutesThatNeedNoRateLimitLoad(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string][]byte{
		"public GET":       spec(authed, thing("get", "[]", "")),
		"authed POST":      spec(authed, thing("post", "", "")),
		"public POST with": spec(authed, thing("post", "[]", "{ip: {rate: 60, per: 1m, burst: 60}}")),
		"public raw POST": spec(authed, strings.Replace(thing("post", "[]", ""), "      responses:",
			"      x-raw-handler: true\n      responses:", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ratelimit.Load(doc); err != nil {
				t.Fatalf("Load = %v", err)
			}
		})
	}
}

func TestLoad_AMalformedExtensionFailsBootWithInvalidConfig(t *testing.T) {
	t.Parallel()
	for name, extension := range map[string]string{
		"no scope":         "{}",
		"null":             "null",
		"not an object":    "[1]",
		"unknown scope":    "{ipv6: {rate: 1, per: 1m, burst: 1}}",
		"unknown field":    "{ip: {rate: 1, per: 1m, burst: 1, cost: 2}}",
		"missing burst":    "{ip: {rate: 1, per: 1m}}",
		"fractional rate":  "{ip: {rate: 1.5, per: 1m, burst: 1}}",
		"unparsable per":   "{actor: {rate: 1, per: soon, burst: 1}}",
		"refill over 24h":  "{actor: {rate: 1, per: 1h, burst: 25}}",
		"one bad of two":   "{actor: {rate: 1, per: 1m, burst: 1}, ip: {rate: 0, per: 1m, burst: 1}}",
		"per as a numeral": "{ip: {rate: 1, per: 60, burst: 1}}",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ratelimit.Load(spec(authed, thing("post", "", extension)))
			if errs.CodeOf(err) != errs.CodeInvalidConfig {
				t.Fatalf("Load with x-rate-limit %s = %v, want invalid_config", extension, err)
			}
		})
	}
}

func TestLoad_AnUnreadableSpecFailsBootWithInvalidConfig(t *testing.T) {
	t.Parallel()
	if _, err := ratelimit.Load([]byte("{")); errs.CodeOf(err) != errs.CodeInvalidConfig {
		t.Fatalf("Load({) = %v, want invalid_config", err)
	}
}
