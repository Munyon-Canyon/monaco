package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func runWithCloseSample(t *testing.T, sampleAt func(closeAt, openAt time.Time) time.Time) (Valuation, string) {
	t.Helper()
	now := valuationTime()
	closeAt := now.Add(-18 * time.Hour)
	openAt := closeAt.Add(-6*time.Hour - 30*time.Minute)
	f, usdc := heldAssetPorts(t, 1)
	closed, open := f.assetRows[0].ID, f.assetRows[1].ID
	f.assetRows[0].Symbol = "AAPLx"
	f.sessions[closed] = market.SessionInfo{State: "closed", LastClose: closeAt, LastSessionOpen: openAt}
	f.asOfAt = func(at time.Time) map[market.AssetID]market.Price {
		if at.Equal(closeAt) {
			return map[market.AssetID]market.Price{
				closed: {Micros: money.MicrosFromUint64(3_000_000), ObservedAt: sampleAt(closeAt, openAt)},
			}
		}
		return map[market.AssetID]market.Price{open: {Micros: money.MicrosFromUint64(1_000_000), ObservedAt: now}}
	}
	logs := &bytes.Buffer{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f}, usdc,
	).Run(ctx, now)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return got, logs.String()
}

func TestRunValuation_aPresentCloseSampleValuesTheCabalWithoutAnAlert(t *testing.T) {
	t.Parallel()
	got, logs := runWithCloseSample(t, func(closeAt, _ time.Time) time.Time { return closeAt.Add(-time.Minute) })
	if len(got.Cabals) != 1 || len(got.Flagged) != 0 || got.Cabals[0].Value != money.MicrosFromUint64(5_000_000) {
		t.Fatalf("Run() = %#v, want 1 USDC + 3 at the close + 1 open", got)
	}
	if strings.Contains(logs, "ranking.close_sample.missing") {
		t.Fatalf("logs = %s, want no missing close sample alert", logs)
	}
}

func TestRunValuation_aMissingCloseSampleFallsBackToTheLastSessionAndAlerts(t *testing.T) {
	t.Parallel()
	got, logs := runWithCloseSample(t, func(closeAt, _ time.Time) time.Time { return closeAt.Add(-2 * time.Hour) })
	if len(got.Cabals) != 1 || len(got.Flagged) != 0 || got.Cabals[0].Value != money.MicrosFromUint64(5_000_000) {
		t.Fatalf("Run() = %#v, want the cabal valued with the last session's sample", got)
	}
	for _, want := range []string{
		`"level":"ERROR"`, `"msg":"ranking.close_sample.missing"`, `"asset":"AAPLx"`, `"code":"ranking_close_sample_missing"`, `"alert":true`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs = %s, want %s", logs, want)
		}
	}
	if strings.Count(logs, "ranking.close_sample.missing") != 1 {
		t.Fatalf("logs = %s, want one alert per asset per run", logs)
	}
}

func TestRunValuation_aSampleFromAnEarlierSessionStaysFlaggedWithoutAnAlert(t *testing.T) {
	t.Parallel()
	got, logs := runWithCloseSample(t, func(_, openAt time.Time) time.Time { return openAt.Add(-24 * time.Hour) })
	if len(got.Cabals) != 0 || len(got.Flagged) != 1 {
		t.Fatalf("Run() = %#v, want the cabal flagged and not valued", got)
	}
	if strings.Contains(logs, "ranking.close_sample.missing") {
		t.Fatalf("logs = %s, want no alert when no sample lies in the last session", logs)
	}
}
