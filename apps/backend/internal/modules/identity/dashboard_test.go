package identity_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDashboard_StatusCounts_GroupsUsersByAuthStateAndAccountStatus(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	for _, u := range []testkit.UserOpts{
		{},
		{},
		{AuthState: "ONBOARDING_COMPLETED"},
		{AuthState: "ONBOARDING_COMPLETED"},
		{AuthState: "ONBOARDING_COMPLETED", AccountStatus: "banned"},
		{AccountStatus: "suspended"},
	} {
		testkit.SeedUser(t, pool, u)
	}
	got, err := identity.New(module.Deps{Pool: pool}).DashboardOn(pool).StatusCounts(t.Context())
	want := []port.StatusCount{
		{AuthState: "CREATED", AccountStatus: "active", Users: 2},
		{AuthState: "CREATED", AccountStatus: "suspended", Users: 1},
		{AuthState: "ONBOARDING_COMPLETED", AccountStatus: "active", Users: 2},
		{AuthState: "ONBOARDING_COMPLETED", AccountStatus: "banned", Users: 1},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("StatusCounts() = %+v, %v, want %+v", got, err, want)
	}
}

func TestDashboard_StatusCounts_FailOnACancelledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := identity.New(module.Deps{Pool: pool}).DashboardOn(pool).StatusCounts(ctx); err == nil {
		t.Fatal("StatusCounts() error = nil on a cancelled context")
	}
}
