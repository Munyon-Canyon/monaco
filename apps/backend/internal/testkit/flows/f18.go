package flows

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (defined) WorkerEnvF18() []string {
	return []string{"MARKET_PRICE_POLL_INTERVAL=2s", "MONACO_TIMEOUT_JUPITER_QUOTE=1s"}
}

const (
	priceRoute   = "/jupiter/price/v3"
	pricePoller  = "market.prices"
	priceRepeats = 4
	aaplMicros   = 254_371_234
	tslaMicros   = 436_120_500
	spaceXMicros = 111_000_000
)

const spaceXMint = "TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v"

var errUnexpectedPriceTick = errors.New("unexpected price tick")

func F18SamplePricesOK(s *scenario.Scenario) {
	var (
		before  time.Time
		scanned int
	)
	s.Given(
		scenario.AwaitTick("market.catalog"),
		ensureSamplerCatalog(),
		countAssets(&scanned),
		scenario.FakeUpstream(fakes.Step{
			Route: priceRoute, Action: fakes.ActionSucceed, Fixture: priceRoute + "/catalog",
			Times: 1, Reset: true,
		}),
		scenario.SubscribeCore(string(events.TypePriceTick)),
	).When(
		scenario.AwaitTick(pricePoller),
		func(s *scenario.Scenario) { scenario.ExpectTick(pricePoller, scanned, 3)(s) },
		expectPriceTick(3),
		deletePricePoints(),
		seedSpaceXReference(),
		captureBucket(&before),
		scenario.FakeUpstream(fakes.Step{
			Route: priceRoute, Action: fakes.ActionSucceed, Fixture: priceRoute + "/moved",
			Times: priceRepeats,
		}),
		scenario.AwaitTick(pricePoller),
		func(s *scenario.Scenario) { scenario.ExpectTick(pricePoller, scanned, 3)(s) },
		expectPriceTick(3),
		expectPricePointsAfter(&before,
			storedPrice{marketfake.AAPLx().Mint.String(), aaplMicros},
			storedPrice{marketfake.TSLAx().Mint.String(), tslaMicros},
			storedPrice{spaceXMint, spaceXMicros},
		),
		scenario.ExpectAllEvents(events.TypeAssetPriceMoved, 2),
		scenario.ExpectLogs("market.price_moved", 2),
	)
}

func F18SamplePricesJupiterUnavailable(s *scenario.Scenario) {
	var bucket time.Time
	s.Given(
		ensureSamplerCatalog(),
		deletePricePoints(),
		seedSamplerPrices(&bucket),
		scenario.FakeUpstream(fakes.Step{
			Route: priceRoute, Action: fakes.ActionFail, Status: 500, Times: priceRepeats, Reset: true,
		}),
	).When(
		scenario.AwaitTick(pricePoller),
		scenario.AwaitTick(pricePoller),
		scenario.ExpectTickFailed(pricePoller, string(errs.CodeJupiterUnavailable)),
		expectPricePointsAt(bucket,
			storedPrice{marketfake.AAPLx().Mint.String(), aaplMicros},
			storedPrice{marketfake.TSLAx().Mint.String(), tslaMicros},
		),
	)
}

func F18SamplePricesUpstreamTimeout(s *scenario.Scenario) {
	s.Given(
		ensureSamplerCatalog(),
		scenario.FakeUpstream(fakes.Step{
			Route: priceRoute, Action: fakes.ActionHang, Times: priceRepeats, Reset: true,
		}),
	).When(
		scenario.AwaitTick(pricePoller),
		deletePricePoints(),
		scenario.AwaitTick(pricePoller),
		scenario.ExpectTickFailed(pricePoller, string(errs.CodeUpstreamTimeout)),
		expectPricePoints(),
	)
}

func F18SamplePricesCrashBeforeCommit(s *scenario.Scenario) {
	var (
		before  time.Time
		scanned int
	)
	s.Given(
		scenario.MarkTick(pricePoller),
		scenario.AwaitTick("market.catalog"),
		ensureSamplerCatalog(),
		countAssets(&scanned),
		scenario.FakeUpstream(fakes.Step{
			Route: priceRoute, Action: fakes.ActionSucceed, Fixture: priceRoute + "/catalog",
			Times: priceRepeats, Reset: true,
		}),
		scenario.SubscribeCore(string(events.TypePriceTick)),
	).When(
		captureBucket(&before),
		scenario.PublishCrashingAt(faultpoint.BeforeCommit),
		scenario.AwaitMarkedTickAfterCrash(pricePoller, faultpoint.BeforeCommit),
		func(s *scenario.Scenario) { scenario.ExpectTick(pricePoller, scanned, 3)(s) },
		expectPriceTick(3),
		expectPricePointsAfter(&before,
			storedPrice{marketfake.AAPLx().Mint.String(), aaplMicros},
			storedPrice{marketfake.TSLAx().Mint.String(), tslaMicros},
			storedPrice{spaceXMint, 100_000_000},
		),
	)
}

func expectPriceTick(prices int) scenario.Step {
	return scenario.ExpectCore(string(events.TypePriceTick), func(data []byte) error {
		var tick events.PriceTick
		if err := json.Unmarshal(data, &tick); err != nil {
			return fmt.Errorf("decode price tick: %w", err)
		}
		if tick.V != 1 || len(tick.Prices) != prices {
			return fmt.Errorf(
				"%w: v%d with %d prices, want v1 with %d", errUnexpectedPriceTick, tick.V, len(tick.Prices), prices,
			)
		}
		return nil
	})
}

func ensureSamplerCatalog() scenario.Step {
	return func(s *scenario.Scenario) {
		now := time.Now().UTC()
		for _, a := range append(marketfake.Fixtures(), marketfake.TSpaceX()) {
			_, err := s.DB().Exec(s.Context(), `INSERT INTO assets (
				id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, company_key,
				first_seen_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT DO NOTHING`,
				a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind),
				a.DisplayName, a.IssuerTradable, a.CompanyKey, now, now)
			if err != nil {
				s.Fatalf("flows: seed %s: %v", a.Symbol, err)
			}
		}
	}
}

func seedSamplerPrices(at *time.Time) scenario.Step {
	return func(s *scenario.Scenario) {
		bucket := domain.Bucket(time.Now().UTC())
		*at = bucket
		for _, row := range []storedPrice{
			{marketfake.AAPLx().Mint.String(), aaplMicros},
			{marketfake.TSLAx().Mint.String(), tslaMicros},
		} {
			_, err := s.DB().Exec(s.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (mint, ts) DO NOTHING`, row.mint, bucket, row.micros, string(domain.SourceJupiter))
			if err != nil {
				s.Fatalf("flows: seed price %s: %v", row.mint, err)
			}
		}
	}
}

func seedSpaceXReference() scenario.Step {
	return func(s *scenario.Scenario) {
		now := time.Now().UTC()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
			VALUES ($1, $2, $3, $4)`, spaceXMint, start, 100_000_000, string(domain.SourceJupiter)); err != nil {
			s.Fatalf("flows: seed SpaceX reference: %v", err)
		}
	}
}

func deletePricePoints() scenario.Step {
	return func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(s.Context(), `DELETE FROM price_points`); err != nil {
			s.Fatalf("flows: delete price_points: %v", err)
		}
	}
}

func countAssets(dst *int) scenario.Step {
	return func(s *scenario.Scenario) {
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM assets`).Scan(dst); err != nil {
			s.Fatalf("flows: count assets: %v", err)
		}
	}
}

func captureBucket(dst *time.Time) scenario.Step {
	return func(*scenario.Scenario) { *dst = domain.Bucket(time.Now().UTC()) }
}

type storedPrice struct {
	mint   string
	micros int64
}

func expectPricePoints(want ...storedPrice) scenario.Step {
	return expectPricePointsAt(domain.Bucket(time.Now().UTC()), want...)
}

func expectPricePointsAt(bucket time.Time, want ...storedPrice) scenario.Step {
	return func(s *scenario.Scenario) {
		rows, err := s.DB().Query(s.Context(),
			`SELECT mint, ts, price_micros, source FROM price_points ORDER BY mint COLLATE "C"`)
		if err != nil {
			s.Fatalf("flows: read price_points: %v", err)
		}
		defer rows.Close()
		var got []storedPrice
		for rows.Next() {
			var row storedPrice
			var ts time.Time
			var source string
			if err := rows.Scan(&row.mint, &ts, &row.micros, &source); err != nil {
				s.Fatalf("flows: scan price_points: %v", err)
			}
			if source != string(domain.SourceJupiter) || !ts.Equal(bucket) {
				s.Fatalf("flows: price_points row %s at %s from %s, want %s at %s",
					row.mint, ts, source, domain.SourceJupiter, bucket)
			}
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			s.Fatalf("flows: read price_points: %v", err)
		}
		slices.SortFunc(want, func(a, b storedPrice) int { return cmp.Compare(a.mint, b.mint) })
		if !slices.Equal(got, want) {
			s.Fatalf("flows: price_points = %+v, want %+v", got, want)
		}
	}
}

func expectPricePointsBetween(before time.Time, want ...storedPrice) scenario.Step {
	return func(s *scenario.Scenario) {
		after := domain.Bucket(time.Now().UTC())
		rows, err := s.DB().Query(s.Context(),
			`SELECT mint, ts, price_micros, source FROM price_points WHERE ts >= $1 ORDER BY mint COLLATE "C"`, before)
		if err != nil {
			s.Fatalf("flows: read price_points: %v", err)
		}
		defer rows.Close()
		var (
			bucket time.Time
			got    []storedPrice
		)
		for rows.Next() {
			var row storedPrice
			var ts time.Time
			var source string
			if err := rows.Scan(&row.mint, &ts, &row.micros, &source); err != nil {
				s.Fatalf("flows: scan price_points: %v", err)
			}
			if source != string(domain.SourceJupiter) || !inBucketRange(ts, before, after, bucket) {
				s.Fatalf("flows: price_points row %s at %s from %s, want one bucket from %s through %s",
					row.mint, ts, source, before, after)
			}
			bucket = ts
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			s.Fatalf("flows: read price_points: %v", err)
		}
		slices.SortFunc(want, func(a, b storedPrice) int { return cmp.Compare(a.mint, b.mint) })
		if !slices.Equal(got, want) {
			s.Fatalf("flows: price_points = %+v, want %+v", got, want)
		}
	}
}

func expectPricePointsAfter(before *time.Time, want ...storedPrice) scenario.Step {
	return func(s *scenario.Scenario) { expectPricePointsBetween(*before, want...)(s) }
}

func inBucketRange(ts, before, after, bucket time.Time) bool {
	return !ts.Before(before) && !ts.After(after) && (bucket.IsZero() || ts.Equal(bucket))
}
