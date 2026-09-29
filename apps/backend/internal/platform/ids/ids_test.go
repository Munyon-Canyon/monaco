package ids_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const validV7 = "01890a5d-ac96-774b-bcce-b302099a8057"

type fixedGen struct{ u uuid.UUID }

func (g fixedGen) NewV7() uuid.UUID { return g.u }

func TestRealGeneratesDistinctV7(t *testing.T) {
	t.Parallel()
	var g ids.Generator = ids.Real{}
	a, b := ids.New[struct{}](g), ids.New[struct{}](g)
	if a == b || a.IsZero() {
		t.Fatalf("two generated ids = %s, %s; want distinct and non-zero", a, b)
	}
	if _, err := ids.ParseUserID(a.String()); err != nil {
		t.Fatalf("generated id %s does not parse as v7: %v", a, err)
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	id, err := ids.ParseCabalID(validV7)
	if err != nil || id.String() != validV7 || id.IsZero() {
		t.Fatalf("ParseCabalID(%q) = %s, %v", validV7, id, err)
	}
	invalid := []string{
		"",
		"00000000-0000-0000-0000-000000000000",
		"f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"01890a5d-ac96-774b-0cce-b302099a8057",
		"01890A5D-AC96-774B-BCCE-B302099A8057",
		"{01890a5d-ac96-774b-bcce-b302099a8057}",
		"urn:uuid:01890a5d-ac96-774b-bcce-b302099a8057",
		"01890a5dac96774bbcceb302099a8057",
		"not-a-uuid",
	}
	for _, raw := range invalid {
		id, err := ids.ParseEventID(raw)
		if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || !id.IsZero() {
			t.Fatalf("ParseEventID(%q) = %s, %v; want zero and invalid_input", raw, id, err)
		}
	}
}

func TestEventIDFromKeepsTheUUID(t *testing.T) {
	t.Parallel()
	u := ids.Real{}.NewV7()
	if got := ids.EventIDFrom(u); got.String() != u.String() || got.IsZero() {
		t.Fatalf("EventIDFrom(%s) = %s", u, got)
	}
}

func TestZeroValueIsInvalid(t *testing.T) {
	t.Parallel()
	var id ids.UserID
	if !id.IsZero() {
		t.Fatal("zero UserID reports non-zero")
	}
	if _, err := json.Marshal(struct {
		ID ids.UserID `json:"id"`
	}{id}); err == nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("json.Marshal(zero id) err = %v, want internal", err)
	}
	if _, err := id.Value(); err == nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("zero id Value() err = %v, want internal", err)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	u, err := uuid.Parse(validV7)
	if err != nil {
		t.Fatal(err)
	}
	want := ids.New[struct{}](fixedGen{u: u})
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `"`+validV7+`"` {
		t.Fatalf("json = %s, want %q", raw, validV7)
	}
	var got ids.ID[struct{}]
	if err := json.Unmarshal(raw, &got); err != nil || got != want {
		t.Fatalf("Unmarshal(%s) = %s, %v", raw, got, err)
	}
	if err := json.Unmarshal([]byte(`"f47ac10b-58cc-4372-a567-0e02b2c3d479"`), &got); err == nil {
		t.Fatal("Unmarshal accepted a v4 id")
	}
	if got != want {
		t.Fatalf("failed Unmarshal changed the id to %s", got)
	}
}

func TestSQLRoundTrip(t *testing.T) {
	t.Parallel()
	want, err := ids.ParseUserID(validV7)
	if err != nil {
		t.Fatal(err)
	}
	v, err := want.Value()
	if err != nil || v != validV7 {
		t.Fatalf("Value() = %v, %v", v, err)
	}
	var fromString, fromBytes ids.UserID
	if err := fromString.Scan(v); err != nil || fromString != want {
		t.Fatalf("Scan(string) = %s, %v", fromString, err)
	}
	if err := fromBytes.Scan([]byte(validV7)); err != nil || fromBytes != want {
		t.Fatalf("Scan([]byte) = %s, %v", fromBytes, err)
	}
	for _, src := range []any{nil, int64(1), "not-a-uuid", []byte("f47ac10b-58cc-4372-a567-0e02b2c3d479")} {
		got := want
		if err := got.Scan(src); errs.CodeOf(err) != errs.CodeDecodeFailed || err == nil || got != want {
			t.Fatalf("Scan(%#v) = %s, %v; want unchanged and decode_failed", src, got, err)
		}
	}
}

func TestCabalIDIsNotAssignableToUserID(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go vet on fixtures; CI runs it without -short, outside the package budget")
	}
	match, err := exec.CommandContext(t.Context(), "go", "vet", "./testdata/match").CombinedOutput()
	if err != nil {
		t.Fatalf("control fixture passing a UserID failed go vet: %v\n%s", err, match)
	}
	out, err := exec.CommandContext(t.Context(), "go", "vet", "./testdata/mismatch").CombinedOutput()
	if err == nil {
		t.Fatal("go vet accepted a CabalID passed as a UserID")
	}
	if !strings.Contains(string(out), "cannot use cabalID") {
		t.Fatalf("go vet failed for another reason:\n%s", out)
	}
}
