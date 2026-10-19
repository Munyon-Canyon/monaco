package identity_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestParseBio(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ in, want string }{
		"plain":            {"Building in public.", "Building in public."},
		"newline kept":     {"Building in public.\nNYC", "Building in public.\nNYC"},
		"trimmed":          {" \n hello \n ", "hello"},
		"empty":            {"", ""},
		"blank":            {"  \n ", ""},
		"composed":         {"café", "café"},
		"emoji":            {"🚀🚀", "🚀🚀"},
		"exactly 160":      {strings.Repeat("é", 160), strings.Repeat("é", 160)},
		"160 after a trim": {" " + strings.Repeat("a", 160) + " ", strings.Repeat("a", 160)},
	} {
		got, err := domain.ParseBio(tc.in)
		if err != nil || got.String() != tc.want || got.Empty() != (tc.want == "") {
			t.Fatalf("%s: ParseBio(%q) = %q, %v, want %q", name, tc.in, got.String(), err, tc.want)
		}
	}
	for name, in := range map[string]string{
		"161 scalars":       strings.Repeat("a", 161),
		"161 emoji":         strings.Repeat("🚀", 161),
		"tab":               "a\tb",
		"carriage return":   "a\r\nb",
		"nul":               "a\x00b",
		"escape":            "a\x1bb",
		"invalid utf-8":     "a\xffb",
		"next line control": "a\u0085b",
	} {
		if _, err := domain.ParseBio(in); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: ParseBio = %v, want invalid_input", name, err)
		}
	}
}

func TestUpdateProfile_bioIsSetAndReadBackWithItsNewline(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	rec := f.updateProfile(t, u.ID, `{"bio":"Building in public.\nNYC"}`)
	if me := decodeMe(t, rec); rec.Code != http.StatusOK || me.Bio == nil || *me.Bio != "Building in public.\nNYC" ||
		me.DisplayName != "Kai" {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	if me := decodeMe(t, f.getMe(t, u.ID)); me.Bio == nil || *me.Bio != "Building in public.\nNYC" {
		t.Fatalf("GET /v1/me bio = %v", me.Bio)
	}
	got := profileEvents(t, f)
	if len(got) != 1 || !reflect.DeepEqual(got[0].Fields, []string{"bio"}) || got[0].DisplayName != "" {
		t.Fatalf("profile events = %+v", got)
	}
}

func TestUpdateProfile_nullAndEmptyBothClearTheBio(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	for name, clear := range map[string]string{"null": `{"bio":null}`, "empty": `{"bio":""}`, "blank": `{"bio":"  "}`} {
		u := f.seed(t, portSeed{handle: "kai_" + name, name: "Kai", wallet: true})
		if rec := f.updateProfile(t, u.ID, `{"bio":"hello"}`); rec.Code != http.StatusOK {
			t.Fatalf("%s: set = %d %s", name, rec.Code, rec.Body)
		}
		rec := f.updateProfile(t, u.ID, clear)
		if me := decodeMe(t, rec); rec.Code != http.StatusOK || me.Bio != nil || me.DisplayName != "Kai" {
			t.Fatalf("%s: clear = %d %s", name, rec.Code, rec.Body)
		}
		if me := decodeMe(t, f.getMe(t, u.ID)); me.Bio != nil {
			t.Fatalf("%s: GET /v1/me bio = %q, want null", name, *me.Bio)
		}
		if got := bioEvents(t, f, u.ID); got != 2 {
			t.Fatalf("%s: bio events = %d, want the set and the clear", name, got)
		}
	}
}

func bioEvents(t *testing.T, f httpFixture, user ids.UserID) int {
	t.Helper()
	var n int
	for _, event := range profileEvents(t, f) {
		if event.UserID == user.UUID() && reflect.DeepEqual(event.Fields, []string{"bio"}) {
			n++
		}
	}
	return n
}

func TestUpdateProfile_clearingAnEmptyBioIsANoop(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	if rec := f.updateProfile(t, u.ID, `{"bio":null}`); rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	if got := profileEvents(t, f); len(got) != 0 {
		t.Fatalf("profile events = %+v", got)
	}
}

func TestUpdateProfile_aBioThatFailsParsingWritesNothing(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	if rec := f.updateProfile(t, u.ID, `{"bio":"keep"}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	for name, body := range map[string]string{
		"161 scalars":     `{"display_name":"Kai Q","bio":"` + strings.Repeat("a", 161) + `"}`,
		"control":         `{"display_name":"Kai Q","bio":"a\u0007b"}`,
		"tab":             `{"display_name":"Kai Q","bio":"a\tb"}`,
		"not a string":    `{"display_name":"Kai Q","bio":5}`,
		"an object":       `{"display_name":"Kai Q","bio":{}}`,
		"an array of one": `{"display_name":"Kai Q","bio":["a"]}`,
	} {
		rec := f.updateProfile(t, u.ID, body)
		if rec.Code != http.StatusBadRequest || decodeProblem(t, rec).Code != apibase.InvalidInput {
			t.Fatalf("%s: update = %d %s", name, rec.Code, rec.Body)
		}
	}
	me := decodeMe(t, f.getMe(t, u.ID))
	if me.Bio == nil || *me.Bio != "keep" || me.DisplayName != "Kai" {
		t.Fatalf("after the refusals, bio = %v, name = %q", me.Bio, me.DisplayName)
	}
	if got := profileEvents(t, f); len(got) != 1 {
		t.Fatalf("profile events = %+v", got)
	}
}

func TestUpdateProfile_aBioAloneLeavesTheOtherFieldsAlone(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	if rec := f.updateProfile(t, u.ID, `{"share_performance":false}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	rec := f.updateProfile(t, u.ID, `{"bio":"hi"}`)
	if me := decodeMe(t, rec); rec.Code != http.StatusOK || me.SharePerformance || me.DisplayName != "Kai" {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
}

func TestUpdateProfile_aBioWithTheOtherFieldsIsOneEvent(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.seed(t, portSeed{handle: "kai", name: "Kai", wallet: true})
	rec := f.updateProfile(t, u.ID, `{"share_performance":false,"bio":"hi","display_name":"Kai Q"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	if got := profileEvents(t, f); len(got) != 1 ||
		!reflect.DeepEqual(got[0].Fields, []string{"display_name", "bio", "share_performance"}) {
		t.Fatalf("profile events = %+v", got)
	}
}

func TestGetMe_carriesTheBio(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	u := f.seed(t, portSeed{handle: "kai", wallet: true})
	if _, err := f.pool.Exec(
		t.Context(), `UPDATE users SET bio = 'a' || chr(10) || 'b' WHERE id = $1`, u.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetMe(t.Context(), f.pool, u.ID)
	if err != nil || got.Bio != "a\nb" {
		t.Fatalf("GetMe bio = %q, %v", got.Bio, err)
	}
}

func TestUsers_theBioColumnRefusesMoreThan160Characters(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	u := f.seed(t, portSeed{handle: "kai", wallet: true})
	_, err := f.pool.Exec(t.Context(), `UPDATE users SET bio = repeat('a', 161) WHERE id = $1`, u.ID.UUID())
	if err == nil {
		t.Fatal("a 161-character bio was stored")
	}
	if _, err := f.pool.Exec(
		t.Context(), `UPDATE users SET bio = repeat('🚀', 160) WHERE id = $1`, u.ID.UUID(),
	); err != nil {
		t.Fatalf("a 160-character bio was refused: %v", err)
	}
}

func TestPatchMe_aBioThatIsNotAStringIsInvalid(t *testing.T) {
	t.Parallel()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(7).NewV7().String()})
	body := &api.UpdateProfileRequest{Bio: json.RawMessage(`5`)}
	_, err := (adapters.HTTP{}).PatchMe(ctx, api.PatchMeRequestObject{Body: body})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("PatchMe with a numeric bio = %v, want invalid_input", err)
	}
}
