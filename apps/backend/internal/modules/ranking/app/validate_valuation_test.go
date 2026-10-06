package app

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	cabals := []CabalValue{{Value: money.MicrosFromUint64(1)}}
	for name, tc := range map[string]struct {
		valuation Valuation
		refused   bool
	}{
		"entries not built":               {Valuation{Cabals: cabals}, true},
		"valued cabals without any rows":  {Valuation{Cabals: cabals, Entries: []Entry{}}, true},
		"nothing valued and nothing rows": {Valuation{Entries: []Entry{}}, false},
		"cabals exist and all excluded":   {Valuation{Entries: []Entry{}, Excluded: 2}, true},
		"all excluded but one flagged":    {Valuation{Entries: []Entry{{}}, Excluded: 2, Flagged: cabals}, false},
		"valued cabals with rows":         {Valuation{Cabals: cabals, Entries: []Entry{{}}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := validate(tc.valuation)
			if tc.refused != (errs.CodeOf(err) == errs.CodeInvalidInput) || (!tc.refused && err != nil) {
				t.Fatalf("validate() = %v, refused want %v", err, tc.refused)
			}
		})
	}
}

func TestPersistQueries_refusesAnUnbuiltValuationBeforeTheDelete(t *testing.T) {
	t.Parallel()
	queries := &snapshotQueriesFake{deleteErr: errs.New(errs.CodeInternal, "delete reached")}
	err := (SnapshotWriter{}).persistQueries(
		t.Context(), queries, &eventAppenderFake{}, uuid.Nil, Valuation{}, valuationTime(), valuationTime(),
	)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("persistQueries() = %v, want the guard to refuse before the delete", err)
	}
}

func TestPersistQueries_refusesToDropAFlaggedCabalsPreviousRowsBeforeTheDelete(t *testing.T) {
	t.Parallel()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	previous := []sqlc.LeaderboardEntry{{Board: "cabals", SubjectID: cabalID.UUID()}}
	for name, tc := range map[string]struct {
		queries *snapshotQueriesFake
		entries []Entry
		want    errs.Code
	}{
		"rows dropped": {
			&snapshotQueriesFake{previous: previous, deleteErr: errs.New(errs.CodeInternal, "delete")},
			[]Entry{{Board: "people"}},
			errs.CodeInvalidInput,
		},
		"read fails": {&snapshotQueriesFake{previousErr: errs.New(errs.CodeInternal, "read")}, []Entry{{}}, errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			valuation := Valuation{Flagged: []CabalValue{{CabalID: cabalID}}, Entries: tc.entries}
			err := (SnapshotWriter{}).persistQueries(
				t.Context(), tc.queries, &eventAppenderFake{}, uuid.Nil, valuation, valuationTime(), valuationTime(),
			)
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("persistQueries() = %v, want code %q", err, tc.want)
			}
		})
	}
}
