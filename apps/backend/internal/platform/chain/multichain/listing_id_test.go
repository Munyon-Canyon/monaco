package multichain_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const seededListing = "b9103e13-4f65-59a1-a6c9-90159264e669"

func TestListingID_roundTripsASeededV5IDThroughScanValueAndJSON(t *testing.T) {
	t.Parallel()
	seeded, err := uuid.Parse(seededListing)
	if err != nil {
		t.Fatal(err)
	}
	want := multichain.ListingIDFrom(seeded)
	if want.UUID() != seeded || want.IsZero() {
		t.Fatalf("ListingIDFrom(%s) = %s", seeded, want)
	}
	wantScans(t, want, seeded)
	if v, err := want.Value(); err != nil || v != seededListing {
		t.Fatalf("Value = %v, %v, want %s", v, err, seededListing)
	}
	wantJSONRoundTrip(t, want)
}

func wantScans(t *testing.T, want multichain.ListingID, seeded uuid.UUID) {
	t.Helper()
	for name, src := range map[string]any{
		"text":  seededListing,
		"bytes": []byte(seededListing),
		"raw":   seeded[:],
	} {
		var got multichain.ListingID
		if err := got.Scan(src); err != nil || got != want {
			t.Fatalf("Scan(%s) = %s, %v, want %s", name, got, err, seededListing)
		}
	}
}

func wantJSONRoundTrip(t *testing.T, want multichain.ListingID) {
	t.Helper()
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte("01928a6e-0000-7000-8000-000000000001")); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[multichain.ListingID]multichain.Listing{want: {ID: want, StockID: stock}})
	if err != nil {
		t.Fatal(err)
	}
	var back map[multichain.ListingID]multichain.Listing
	if err := json.Unmarshal(raw, &back); err != nil || back[want].ID != want {
		t.Fatalf("JSON round trip of %s = %+v, %v", raw, back, err)
	}
}

func TestListingID_refusesWhatIsNotAUUID(t *testing.T) {
	t.Parallel()
	var id multichain.ListingID
	if err := id.UnmarshalText([]byte("AAPLx")); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("UnmarshalText = %v, want invalid_input", err)
	}
	if err := id.Scan(42); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Scan(42) = %v, want decode_failed", err)
	}
	if _, err := (multichain.ListingID{}).Value(); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("zero Value = %v, want internal", err)
	}
}
