package identity_test

import (
	"sync"
	"testing"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithNATS())
}

var (
	parsedContract = sync.OnceValues(func() (*httpx.Contract, error) { return httpx.LoadContract(openapi.Spec) })
	parsedPolicies = sync.OnceValues(func() (ratelimit.Policies, error) { return ratelimit.Load(openapi.Spec) })
)

func specContract(t *testing.T) *httpx.Contract {
	t.Helper()
	c, err := parsedContract()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func specPolicies(t *testing.T) ratelimit.Policies {
	t.Helper()
	p, err := parsedPolicies()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
