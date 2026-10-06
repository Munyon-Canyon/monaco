package social_test

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
	socialContractOnce = sync.OnceValues(func() (*httpx.Contract, error) { return httpx.LoadContract(openapi.Spec) })
	socialPoliciesOnce = sync.OnceValues(func() (ratelimit.Policies, error) { return ratelimit.Load(openapi.Spec) })
)
