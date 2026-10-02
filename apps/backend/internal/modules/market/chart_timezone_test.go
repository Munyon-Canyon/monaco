package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestChartQueries_bucketFromAUTCOriginOutsideAUTCSession(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	at := time.Date(2000, 1, 1, 0, 10, 0, 0, time.UTC)
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		VALUES ($1, $2, 100000000, 'jupiter')`, mint, at); err != nil {
		t.Fatal(err)
	}
	conn, err := s.pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(t.Context(), `SET TIME ZONE 'America/New_York'`); err != nil {
		t.Fatal(err)
	}
	rows, err := sqlc.New(conn).ChartBuckets(t.Context(), sqlc.ChartBucketsParams{
		BucketSeconds: 86400, Mint: mint,
		Since: at.Add(-time.Hour), Until: at.Add(time.Hour),
	})
	if err != nil || len(rows) != 1 || !rows[0].Bucket.Equal(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("ChartBuckets = %+v, %v", rows, err)
	}
}
