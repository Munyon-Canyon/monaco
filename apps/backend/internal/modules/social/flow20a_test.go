package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow20a_BlockUser_OK(t *testing.T) {
	t.Parallel()
	flows.F20aBlockUserOK(scenario.New(t, withSocial()))
}

func TestFlow20a_BlockUser_CannotBlockSelf(t *testing.T) {
	t.Parallel()
	flows.F20aBlockUserCannotBlockSelf(scenario.New(t, withSocial()))
}

func TestFlow20a_BlockUser_UserNotFound(t *testing.T) {
	t.Parallel()
	flows.F20aBlockUserUserNotFound(scenario.New(t, withSocial()))
}

func TestFlow20a_BlockUser_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F20aBlockUserUnauthorized(scenario.New(t, withSocial()))
}

func TestFlow20a_UnblockUser_OK(t *testing.T) {
	t.Parallel()
	flows.F20aUnblockUserOK(scenario.New(t, withSocial()))
}

func TestFlow20a_CreateReport_OK(t *testing.T) {
	t.Parallel()
	flows.F20aCreateReportOK(scenario.New(t, withSocial()))
}

func TestFlow20a_CreateReport_ReportTargetNotFound(t *testing.T) {
	t.Parallel()
	flows.F20aCreateReportReportTargetNotFound(scenario.New(t, withSocial()))
}

func TestFlow20a_CreateReport_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F20aCreateReportRateLimited(scenario.New(t, withSocial()))
}
