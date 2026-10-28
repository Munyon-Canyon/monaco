package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSampleOf_carriesTheRowSource(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	for _, source := range []string{"jupiter", "coingecko", ""} {
		row := sqlc.RecentPriceSamplesRow{AssetID: testkit.NewIDs(1).NewV7(), Ts: at, PriceMicros: 5, Source: source}
		_, got, err := sampleOf(row)
		if err != nil {
			t.Fatal(err)
		}
		if got.Source != domain.Source(source) {
			t.Fatalf("source = %q, want %q", got.Source, source)
		}
	}
}

func TestGroupNewest_carriesTheRowSource(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	got, err := groupNewest([]sqlc.NewestSamplesRow{
		{Mint: "m", Ts: at, PriceMicros: 5, Source: "coingecko"},
		{Mint: "m", Ts: at.Add(-time.Minute), PriceMicros: 5, Source: "jupiter"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := domain.Accept(got["m"])
	if !ok || accepted.Source != domain.SourceCoinGecko {
		t.Fatalf("accepted = %+v ok=%v, want the coingecko sample", accepted, ok)
	}
}
