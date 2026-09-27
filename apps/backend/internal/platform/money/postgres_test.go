package money_test

import (
	"context"
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPostgresRoundTripThroughNumeric(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	ctx := context.Background()
	if _, err := db.Exec(
		ctx,
		`CREATE TABLE amounts (micros numeric(20,0) NOT NULL, delta numeric(20,0) NOT NULL)`,
	); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		micros money.Micros
		delta  money.SignedMicros
		text   string
	}{
		{money.MicrosFromUint64(math.MaxUint64), money.SignedMicrosFromInt64(math.MinInt64), "18446744073709551615"},
		{money.MicrosFromUint64(1_500_000), money.SignedMicrosFromInt64(-7), "1500000"},
		{money.MicrosFromUint64(0), money.SignedMicrosFromInt64(math.MaxInt64), "0"},
	}
	for _, r := range rows {
		if _, err := db.Exec(ctx, `DELETE FROM amounts`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO amounts VALUES ($1, $2)`, r.micros, r.delta); err != nil {
			t.Fatalf("insert %v, %v: %v", r.micros, r.delta, err)
		}
		var micros money.Micros
		var delta money.SignedMicros
		var text string
		if err := db.QueryRow(ctx, `SELECT micros, delta, micros::text FROM amounts`).
			Scan(&micros, &delta, &text); err != nil {
			t.Fatalf("scan %v, %v: %v", r.micros, r.delta, err)
		}
		if micros != r.micros || delta != r.delta || text != r.text {
			t.Fatalf("round trip = %v, %v, stored %q; want %v, %v, %q", micros, delta, text, r.micros, r.delta, r.text)
		}
	}
}
