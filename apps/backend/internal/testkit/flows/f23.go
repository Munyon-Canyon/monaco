package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func F23UpdateProfileOK(s *scenario.Scenario) {
	s.Given(scenario.SignIn(privyProfileOK)).
		When(scenario.Patch(me, `{"display_name":"Kai Q"}`), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectEvents(events.TypeUserProfileUpdated, 1))
}

func F23UpdateProfileDisplayNameInvalid(s *scenario.Scenario) {
	s.Given(scenario.SignIn(privyProfileInvalid)).
		When(scenario.Patch(me, `{"display_name":" "}`)).
		Then(scenario.ExpectProblem(errs.CodeDisplayNameInvalid))
}

func F23aUploadProfilePhotoOK(s *scenario.Scenario) {
	s.Given(scenario.SignIn(privyPhotoOK)).
		When(scenario.PostPhoto("/v1/me/profile-photo", []byte("\x89PNG\r\n\x1a\nphoto")), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectEvents(events.TypeUserProfileUpdated, 1))
}

func F23aUploadProfilePhotoPhotoInvalid(s *scenario.Scenario) {
	s.Given(scenario.SignIn(privyPhotoInvalid)).
		When(scenario.PostPhoto("/v1/me/profile-photo", []byte("GIF89a"))).
		Then(scenario.ExpectProblem(errs.CodePhotoInvalid))
}

func F23aUploadProfilePhotoStorageUnavailable(s *scenario.Scenario) {
	s.Given(scenario.SignIn(privyPhotoStorage), scenario.FakeUpstream(fakes.Step{
		Route: "/storage/v1/object/avatars", Action: fakes.ActionFail, Status: http.StatusServiceUnavailable, Times: 1,
	})).
		When(scenario.PostPhoto("/v1/me/profile-photo", []byte("\x89PNG\r\n\x1a\nphoto"))).
		Then(scenario.ExpectProblem(errs.CodeStorageUnavailable), scenario.ExpectEvents(events.TypeUserProfileUpdated, 0))
}

func F23aUploadProfilePhotoRateLimited(s *scenario.Scenario) {
	s.Given(scenario.SignIn(privyPhotoRate)).
		When(
			scenario.PostPhoto("/v1/me/profile-photo", []byte("GIF89a")),
			scenario.PostPhoto("/v1/me/profile-photo", []byte("GIF89a")),
			scenario.PostPhoto("/v1/me/profile-photo", []byte("GIF89a")),
			scenario.PostPhoto("/v1/me/profile-photo", []byte("GIF89a")),
		).
		Then(scenario.ExpectProblem(errs.CodeRateLimited))
}
