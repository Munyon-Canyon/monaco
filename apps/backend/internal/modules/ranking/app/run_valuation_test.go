package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func valuationTime() time.Time { return time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC) }

func TestRunValuation_readsEachBatchPortOnce(t *testing.T) {
	t.Parallel()
	f := &countingPorts{}
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		"",
	).Run(t.Context(), valuationTime())
	if err != nil || len(got.Cabals) != 0 || got.Excluded != 0 {
		t.Fatalf("Run() = %#v, %v", got, err)
	}
	wantOneReadPerPort(t, f)
}

func TestRunValuation_readsEachBatchPortOnceForFiveHundredCabals(t *testing.T) {
	t.Parallel()
	f := &countingPorts{cabals: make([]cabal.View, 500)}
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		"",
	).Run(t.Context(), valuationTime())
	if err != nil || got.Excluded != 500 {
		t.Fatalf("Run() = %#v, %v", got, err)
	}
	wantOneReadPerPort(t, f)
}

func TestRunValuation_excludesBannedPausedAndUnconservedCabals(t *testing.T) {
	t.Parallel()
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	for name, setup := range map[string]func(ids.CabalID, *countingPorts){
		"banned": func(_ ids.CabalID, f *countingPorts) { f.cabals[0].Status = cabal.StatusBanned },
		"paused": func(id ids.CabalID, f *countingPorts) {
			f.pausedSet.Cabals = map[ids.CabalID][]funding.PauseReason{id: {"ops"}}
		},
		"conservation broken": func(id ids.CabalID, f *countingPorts) {
			f.positionRows = []treasury.CabalPositions{{CabalID: id, Holdings: []treasury.Position{{Mint: usdc, Units: money.NewBaseUnits(1, 6)}}, TotalShares: money.SharesUnitsFromUint64(1)}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			id := ids.CabalIDFrom(ids.Real{}.NewV7())
			f := &countingPorts{cabals: []cabal.View{{ID: id}}}
			setup(id, f)
			got, err := NewRunValuation(
				Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
				usdc,
			).Run(t.Context(), valuationTime())
			if err != nil || len(got.Cabals) != 0 || got.Excluded != 1 {
				t.Fatalf("Run() = %#v, %v", got, err)
			}
		})
	}
}

func TestRunValuation_returnsReadErrors(t *testing.T) {
	t.Parallel()
	for name, setup := range map[string]func(*countingPorts){
		"cabals":    func(f *countingPorts) { f.allErr = errs.New(errs.CodeInternal, "test") },
		"members":   func(f *countingPorts) { f.membersErr = errs.New(errs.CodeInternal, "test") },
		"positions": func(f *countingPorts) { f.positionsErr = errs.New(errs.CodeInternal, "test") },
		"stakes":    func(f *countingPorts) { f.stakesErr = errs.New(errs.CodeInternal, "test") },
		"stakes at a range start": func(f *countingPorts) {
			f.stakesErr, f.stakesErrAfter = errs.New(errs.CodeInternal, "test"), 3
		},
		"flows":     func(f *countingPorts) { f.flowsErr = errs.New(errs.CodeInternal, "test") },
		"snapshots": func(f *countingPorts) { f.snapshotsErr = errs.New(errs.CodeInternal, "test") },
		"users": func(f *countingPorts) {
			f.userErr = errs.New(errs.CodeInternal, "test")
			f.stakeRows = []treasury.MemberStake{{UserID: ids.UserIDFrom(ids.Real{}.NewV7())}}
		},
		"pauses":       func(f *countingPorts) { f.pausesErr = errs.New(errs.CodeInternal, "test") },
		"catalog":      func(f *countingPorts) { f.assetsErr = errs.New(errs.CodeInternal, "test") },
		"prices as of": func(f *countingPorts) { f.asOfErr = errs.New(errs.CodeInternal, "test") },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := &countingPorts{}
			setup(f)
			if _, err := NewRunValuation(
				Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
				"",
			).Run(t.Context(), valuationTime()); err == nil {
				t.Fatal("Run() error = nil")
			}
		})
	}
}

func TestRunValuation_helpers(t *testing.T) {
	t.Parallel()
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := splitCash(
		[]treasury.Position{{Mint: usdc, Units: money.NewBaseUnits(1, 5)}},
		usdc,
	); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("splitCash wrong decimals = %v", err)
	}
	if _, _, err := splitCash(
		[]treasury.Position{
			{Mint: usdc, Units: money.NewBaseUnits(^uint64(0), 6)},
			{Mint: usdc, Units: money.NewBaseUnits(1, 6)},
		},
		usdc,
	); err == nil {
		t.Fatal("splitCash overflow error = nil")
	}
	price := money.MicrosFromUint64(1)
	id := ids.CabalIDFrom(ids.Real{}.NewV7())
	if err := conservation(
		id,
		money.SharesUnitsFromUint64(1),
		[]treasury.MemberStake{{CabalID: id, ShareUnits: money.SharesUnitsFromUint64(2)}},
		price,
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("conservation invalid stake = %v", err)
	}
	f := &countingPorts{}
	stakes := make([]treasury.MemberStake, 501)
	for i := range stakes {
		stakes[i].UserID = ids.UserIDFrom(ids.Real{}.NewV7())
	}
	if _, err := NewRunValuation(
		Ports{Users: f, Previous: f, Snapshots: f},
		"",
	).readUsers(t.Context(), stakes, nil); err != nil {
		t.Fatalf("readUsers() = %v", err)
	}
}

func TestRunValuation_cabalNAVErrors(t *testing.T) {
	t.Parallel()
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	stock, err := market.ParseMint("So11111111111111111111111111111111111111112")
	if err != nil {
		t.Fatal(err)
	}
	var listed market.AssetID
	now := valuationTime()
	runner := NewRunValuation(Ports{Market: &countingPorts{}}, usdc)
	if _, _, err := runner.cabalNAV(
		money.Micros{},
		money.Micros{},
		[]treasury.Position{{Mint: stock.Address()}},
		money.SharesUnits{},
		nil,
		nil,
		now,
	); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("cabalNAV unknown mint = %v", err)
	}
	assets := map[string]market.Asset{stock.String(): {ID: listed, Mint: stock}}
	if _, flags, err := runner.cabalNAV(
		money.Micros{},
		money.Micros{},
		[]treasury.Position{{Mint: stock.Address()}},
		money.SharesUnits{},
		assets,
		nil,
		now,
	); err != nil ||
		len(flags) != 1 {
		t.Fatalf("cabalNAV unpriced = %v, %v", flags, err)
	}
	fresh := map[market.AssetID]market.Price{listed: {Micros: money.MicrosFromUint64(1), ObservedAt: now}}
	if _, _, err := runner.cabalNAV(
		money.Micros{},
		money.Micros{},
		[]treasury.Position{{Mint: stock.Address(), Units: money.NewBaseUnits(1, 20)}},
		money.SharesUnits{},
		assets,
		fresh,
		now,
	); err == nil {
		t.Fatal("cabalNAV invalid units error = nil")
	}
}

func TestSnapshotWriter_queries(t *testing.T) {
	t.Parallel()
	if NewSnapshotWriter(nil, ids.Real{}).uow != nil {
		t.Fatal("NewSnapshotWriter() did not keep the unit of work")
	}
	fake := &snapshotQueriesFake{}
	writer := SnapshotWriter{}
	value := CabalValue{
		CabalID:     ids.CabalIDFrom(ids.Real{}.NewV7()),
		Value:       money.MicrosFromUint64(1),
		NavPerShare: money.MicrosFromUint64(1),
		TotalShares: money.SharesUnitsFromUint64(1),
	}
	valuation := Valuation{AsOf: valuationTime(), PricesAsOf: valuationTime(), Cabals: []CabalValue{value}}
	if err := writer.snapshots(t.Context(), fake, valuation); err != nil || fake.snapshots != 1 {
		t.Fatalf("snapshots() = %v, calls %d", err, fake.snapshots)
	}
	fake.err = errs.New(errs.CodeInternal, "test")
	if err := writer.snapshots(t.Context(), fake, valuation); err == nil {
		t.Fatal("snapshots() error = nil")
	}
	if err := writer.snapshots(t.Context(), &snapshotQueriesFake{}, Valuation{
		AsOf: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), Cabals: []CabalValue{value},
	}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("snapshots() with an unencodable time = %v", err)
	}
	maxMicros, maxShares := money.MicrosFromUint64(^uint64(0)), money.SharesUnitsFromUint64(^uint64(0))
	for _, overflow := range []CabalValue{{Value: maxMicros}, {NavPerShare: maxMicros}, {TotalShares: maxShares}} {
		if _, err := int64s(overflow); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("int64s(%+v) = %v", overflow, err)
		}
	}
	fake.err = nil
	now, runID := valuationTime(), ids.Real{}.NewV7()
	if err := writer.run(t.Context(), fake, runID, valuation, now, now); err != nil {
		t.Fatalf("run() = %v", err)
	}
	tooMany := Valuation{Excluded: int(^uint(0) >> 1)}
	if err := writer.run(t.Context(), fake, runID, tooMany, now, now); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("run() = %v", err)
	}
}

func TestSnapshotWriter_snapshotsMakeOneStatementForEveryCabal(t *testing.T) {
	t.Parallel()
	fake := &snapshotQueriesFake{}
	valuation := Valuation{AsOf: valuationTime(), PricesAsOf: valuationTime()}
	for i := range 3 {
		valuation.Cabals = append(valuation.Cabals, CabalValue{
			CabalID:     ids.CabalIDFrom(ids.Real{}.NewV7()),
			Value:       money.MicrosFromUint64(uint64(10 + i)),
			NavPerShare: money.MicrosFromUint64(uint64(20 + i)),
			TotalShares: money.SharesUnitsFromUint64(uint64(30 + i)),
		})
	}
	if err := (SnapshotWriter{}).snapshots(t.Context(), fake, valuation); err != nil {
		t.Fatalf("snapshots() = %v", err)
	}
	var rows []snapshotRow
	if err := json.Unmarshal(fake.snapshotRows, &rows); err != nil || fake.snapshots != 1 || len(rows) != 3 {
		t.Fatalf("snapshots() made %d statements with %d rows (%v), want 1 with 3", fake.snapshots, len(rows), err)
	}
	for i, row := range rows {
		cabal := valuation.Cabals[i]
		if row.CabalID != cabal.CabalID.UUID() || !row.At.Equal(valuation.AsOf) || row.ValueMicros != int64(10+i) ||
			row.NavPerShareMicros != int64(20+i) || row.TotalShares != int64(30+i) {
			t.Fatalf("row %d = %+v, want cabal %v at %v", i, row, cabal.CabalID, valuation.AsOf)
		}
	}
}

func TestSnapshotWriter_snapshotsMakeNoStatementWithoutCabals(t *testing.T) {
	t.Parallel()
	fake := &snapshotQueriesFake{}
	if err := (SnapshotWriter{}).snapshots(t.Context(), fake, Valuation{AsOf: valuationTime()}); err != nil ||
		fake.snapshots != 0 {
		t.Fatalf("snapshots() = %v after %d statements, want none", err, fake.snapshots)
	}
}

func TestSnapshotWriter_persistQueriesRejectsAnEntryThatCannotBeEncoded(t *testing.T) {
	t.Parallel()
	valuation := Valuation{Entries: []Entry{{SubjectCreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}}
	if err := (SnapshotWriter{}).persistQueries(
		t.Context(),
		&snapshotQueriesFake{},
		&eventAppenderFake{},
		uuid.Nil,
		valuation,
		valuationTime(),
		valuationTime(),
	); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("persistQueries() = %v", err)
	}
}

func TestInt32s(t *testing.T) {
	t.Parallel()
	if got, err := int32s(7, 9); err != nil || got[0] != 7 || got[1] != 9 {
		t.Fatalf("int32s(7, 9) = %v, %v", got, err)
	}
	if _, err := int32s(7, int(^uint(0)>>1)); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("int32s(max) = %v", err)
	}
}

func TestSnapshotWriter_persistQueriesStopsAtEveryWriteError(t *testing.T) {
	t.Parallel()
	writer := SnapshotWriter{}
	valuation := Valuation{AsOf: valuationTime(), PricesAsOf: valuationTime(), Entries: []Entry{}}
	writeErr := errs.New(errs.CodeInternal, "test")
	for name, configure := range map[string]func(*snapshotQueriesFake, *eventAppenderFake){
		"delete":   func(q *snapshotQueriesFake, _ *eventAppenderFake) { q.deleteErr = writeErr },
		"entries":  func(q *snapshotQueriesFake, _ *eventAppenderFake) { q.entriesErr = writeErr },
		"run":      func(q *snapshotQueriesFake, _ *eventAppenderFake) { q.err = writeErr },
		"triggers": func(q *snapshotQueriesFake, _ *eventAppenderFake) { q.triggersErr = writeErr },
		"event":    func(_ *snapshotQueriesFake, e *eventAppenderFake) { e.err = writeErr },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			queries := &snapshotQueriesFake{}
			appender := &eventAppenderFake{}
			configure(queries, appender)
			if err := writer.persistQueries(
				t.Context(),
				queries,
				appender,
				uuid.Nil,
				valuation,
				valuationTime(),
				valuationTime(),
			); !errors.Is(err, writeErr) {
				t.Fatalf("persistQueries() = %v, want %v", err, writeErr)
			}
		})
	}
	if err := writer.persistQueries(
		t.Context(),
		&snapshotQueriesFake{},
		&eventAppenderFake{},
		uuid.Nil,
		Valuation{Cabals: []CabalValue{{Value: money.MicrosFromUint64(^uint64(0))}}, Entries: []Entry{{}}},
		valuationTime(),
		valuationTime(),
	); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("persistQueries() = %v", err)
	}
}

type snapshotQueriesFake struct {
	err                                error
	deleteErr, entriesErr, triggersErr error
	previousErr                        error
	previous                           []sqlc.LeaderboardEntry
	snapshots                          int
	snapshotRows                       []byte
	entries                            []byte
}

func (f *snapshotQueriesFake) PreviousEntriesForCabals(
	context.Context,
	sqlc.PreviousEntriesForCabalsParams,
) ([]sqlc.LeaderboardEntry, error) {
	return f.previous, f.previousErr
}

func (f *snapshotQueriesFake) DeleteAllLeaderboardEntries(context.Context) error { return f.deleteErr }

func (f *snapshotQueriesFake) InsertLeaderboardEntries(_ context.Context, rows []byte) error {
	f.entries = rows
	return f.entriesErr
}

func (f *snapshotQueriesFake) InsertCabalValueSnapshots(_ context.Context, rows []byte) error {
	f.snapshots++
	f.snapshotRows = rows
	return f.err
}

func (f *snapshotQueriesFake) InsertLeaderboardRun(context.Context, sqlc.InsertLeaderboardRunParams) error {
	return f.err
}

func (f *snapshotQueriesFake) DeleteRankingTriggersThrough(context.Context, time.Time) error {
	return f.triggersErr
}

type eventAppenderFake struct {
	err error
	got events.Event
}

func (f *eventAppenderFake) Append(_ context.Context, event events.Event) error {
	f.got = event
	return f.err
}

func TestRunValuation_rejectsInvalidCashUnits(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 1)
	f.positionRows[0].Holdings[0].Units = money.NewBaseUnits(1, 5)
	_, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).
		Run(t.Context(), valuationTime())
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Run() = %v", err)
	}
}

func TestRunValuation_excludesUnpricedAndAnInconsistentCabal(t *testing.T) {
	t.Parallel()
	now := valuationTime()
	stock, err := market.ParseMint("So11111111111111111111111111111111111111112")
	if err != nil {
		t.Fatal(err)
	}
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	var assetID market.AssetID
	for name, setup := range map[string]func(ids.CabalID, *countingPorts){
		"unpriced": func(id ids.CabalID, f *countingPorts) {
			f.positionRows = []treasury.CabalPositions{{CabalID: id, Holdings: []treasury.Position{{Mint: stock.Address()}}}}
			f.assetRows = []market.Asset{{ID: assetID, Mint: stock}}
		},
		"invalid conservation": func(id ids.CabalID, f *countingPorts) {
			f.positionRows = []treasury.CabalPositions{{CabalID: id, Holdings: []treasury.Position{{Mint: usdc, Units: money.NewBaseUnits(1, 6)}}, TotalShares: money.SharesUnitsFromUint64(1)}}
			f.stakeRows = []treasury.MemberStake{{CabalID: id, ShareUnits: money.SharesUnitsFromUint64(2)}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			id := ids.CabalIDFrom(ids.Real{}.NewV7())
			f := &countingPorts{cabals: []cabal.View{{ID: id}}}
			setup(id, f)
			got, runErr := NewRunValuation(
				Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
				usdc,
			).Run(t.Context(), now)
			if name == "unpriced" && (runErr != nil || got.Excluded != 1) {
				t.Fatalf("Run() = %#v, %v", got, runErr)
			}
			if name == "invalid conservation" && (runErr != nil || got.Excluded != 1) {
				t.Fatalf("Run() = %#v, %v", got, runErr)
			}
		})
	}
}

func TestRunValuation_flagsACabalHoldingAnUncataloguedMintAndValuesTheRest(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 2)
	stray := f.positionRows[0].Holdings[1]
	stray.Mint = "11111111111111111111111111111111"
	f.positionRows[0].Holdings = []treasury.Position{f.positionRows[0].Holdings[0], stray}
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).
		Run(t.Context(), valuationTime())
	if err != nil || len(got.Cabals) != 1 || got.Cabals[0].CabalID != f.cabals[1].ID || len(got.Flagged) != 1 ||
		got.Flagged[0].CabalID != f.cabals[0].ID || got.Flagged[0].Flags[0] != domain.FlagUnpricedAssets ||
		got.Excluded != 1 {
		t.Fatalf("Run() = %#v, %v, want the healthy cabal valued and the stray holder flagged", got, err)
	}
}

func TestRunValuation_flagsACabalTreasuryCouldNotPrice(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 1)
	f.positionRows[0].Unpriced = true
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).
		Run(t.Context(), valuationTime())
	if err != nil || len(got.Cabals) != 0 || len(got.Flagged) != 1 {
		t.Fatalf("Run() = %#v, %v, want the cabal flagged and the tick to succeed", got, err)
	}
}

func TestRunValuation_valuesUSDCWithoutCatalogLookup(t *testing.T) {
	t.Parallel()
	now := valuationTime()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	userID := ids.UserIDFrom(ids.Real{}.NewV7())
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	f := &countingPorts{
		cabals:  []cabal.View{{ID: cabalID}},
		members: map[ids.CabalID][]cabal.MemberView{cabalID: {{UserID: userID}}},
		positionRows: []treasury.CabalPositions{
			{
				CabalID:     cabalID,
				Holdings:    []treasury.Position{{Mint: usdc, Units: money.NewBaseUnits(2_000_000, 6)}},
				TotalShares: money.SharesUnitsFromUint64(1_000_000),
			},
		},
		stakeRows: []treasury.MemberStake{
			{CabalID: cabalID, UserID: userID, ShareUnits: money.SharesUnitsFromUint64(1_000_000)},
		},
	}
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), now)
	if err != nil || len(got.Cabals) != 1 || got.Cabals[0].Value != money.MicrosFromUint64(2_000_000) {
		t.Fatalf("Run() = %#v, %v", got, err)
	}
	if f.assets != 1 || f.asOf != 1 {
		t.Fatalf("market calls = %#v", f)
	}
}

func newAssetID(t *testing.T) market.AssetID {
	t.Helper()
	var id market.AssetID
	if err := id.UnmarshalText([]byte(ids.Real{}.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	return id
}

func heldAssetPorts(t *testing.T, cabalCount int) (*countingPorts, chain.SolanaAddress) {
	t.Helper()
	now := valuationTime()
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	mints := make([]market.Mint, 2)
	for i, raw := range []string{
		"So11111111111111111111111111111111111111112", "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
	} {
		if mints[i], err = market.ParseMint(raw); err != nil {
			t.Fatal(err)
		}
	}
	f := &countingPorts{
		assetRows: []market.Asset{{ID: newAssetID(t), Mint: mints[0]}},
		asOfRows:  map[market.AssetID]market.Price{},
	}
	f.assetRows = append(f.assetRows, market.Asset{ID: newAssetID(t), Mint: mints[1]})
	for _, asset := range f.assetRows {
		price := market.Price{Micros: money.MicrosFromUint64(1_000_000), ObservedAt: now}
		f.asOfRows[asset.ID] = price
	}
	for range cabalCount {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		f.cabals = append(f.cabals, cabal.View{ID: id})
		f.positionRows = append(f.positionRows, treasury.CabalPositions{
			CabalID: id,
			Holdings: []treasury.Position{
				{Mint: usdc, Units: money.NewBaseUnits(1_000_000, 6)},
				{Mint: mints[0].Address(), Units: money.NewBaseUnits(1, 0)},
				{Mint: mints[1].Address(), Units: money.NewBaseUnits(1, 0)},
			},
			TotalShares: money.SharesUnitsFromUint64(1),
		})
		f.stakeRows = append(f.stakeRows, treasury.MemberStake{
			CabalID: id, UserID: ids.UserIDFrom(ids.Real{}.NewV7()), ShareUnits: money.SharesUnitsFromUint64(1),
		})
	}
	return f, usdc
}

func TestRunValuation_readsPricesOncePerRunWhateverTheCabalCount(t *testing.T) {
	t.Parallel()
	for _, cabals := range []int{1, 500} {
		f, usdc := heldAssetPorts(t, cabals)
		got, err := NewRunValuation(
			Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
			usdc,
		).Run(t.Context(), valuationTime())
		if err != nil || len(got.Cabals) != cabals || len(got.Flagged) != 0 {
			t.Fatalf("Run() with %d cabals = %d valued, %d flagged, %v", cabals, len(got.Cabals), len(got.Flagged), err)
		}
		if f.asOf != 1 || f.assets != 1 || f.positions != 1 {
			t.Fatalf("%d cabals: calls = %#v", cabals, f)
		}
	}
}

func TestRunValuation_asksTheCatalogOnlyForTheMintsACabalHolds(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 1)
	unheld, err := market.ParseMint("4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R")
	if err != nil {
		t.Fatal(err)
	}
	f.assetRows = append(f.assetRows, market.Asset{ID: newAssetID(t), Mint: unheld})
	f.positionRows[0].Holdings = f.positionRows[0].Holdings[:2]
	held := string(f.positionRows[0].Holdings[1].Mint)
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), valuationTime())
	if err != nil || len(got.Cabals) != 1 || len(f.assetRows) != 3 {
		t.Fatalf("Run() = %#v, %v", got, err)
	}
	if want := [][]string{{held}}; !slices.EqualFunc(f.assetMints, want, slices.Equal) {
		t.Fatalf("catalog asked for %v, want exactly %v of 3 assets", f.assetMints, want)
	}
}

func TestRunValuation_returnsAValuationErrorFromTheStage(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 100)
	f.positionRows[0].Holdings[1].Units = money.NewBaseUnits(1, 20)
	if _, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), valuationTime()); err == nil {
		t.Fatal("Run() error = nil, want the stage error")
	}
}

func TestRunValuation_flagsACabalWithAnUnpricedAssetInsteadOfDroppingIt(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 2)
	delete(f.asOfRows, f.assetRows[0].ID)
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), valuationTime())
	if err != nil || len(got.Cabals) != 0 || len(got.Flagged) != 2 || got.Excluded != 2 {
		t.Fatalf("Run() = %#v, %v", got, err)
	}
	if len(got.Flagged[0].Flags) == 0 {
		t.Fatalf("flagged cabal carries no flags: %#v", got.Flagged[0])
	}
}

type countingPorts struct {
	all, memberCalls, positions, stakes, paused, assets, asOf             int
	reservations, previousCalls, flowCalls, snapshotCalls, stakesErrAfter int
	flowRows                                                              []treasury.MemberFlow
	snapshotRows                                                          []sqlc.SnapshotsAtRow
	flowsErr, snapshotsErr                                                error
	reservedRows                                                          map[ids.CabalID]money.Micros
	userRows                                                              map[ids.UserID]identity.UserCard
	previousRows                                                          []sqlc.LeaderboardEntry
	reservedErr, previousErr                                              error
	cabals                                                                []cabal.View
	members                                                               map[ids.CabalID][]cabal.MemberView
	positionRows                                                          []treasury.CabalPositions
	stakeRows                                                             []treasury.MemberStake
	assetRows                                                             []market.Asset
	assetMints                                                            [][]string
	asOfInstants                                                          []time.Time
	asOfAt                                                                func(time.Time) map[market.AssetID]market.Price
	asOfRows                                                              map[market.AssetID]market.Price
	pausedSet                                                             funding.PausedSet
	allErr, membersErr, positionsErr, stakesErr                           error
	userErr, pausesErr, assetsErr, asOfErr                                error
	listingRows                                                           map[market.ListingKey]market.ListingPrice
	listingsErr                                                           error
	listingCalls                                                          int
	inputsAt                                                              []time.Time
}

func (f *countingPorts) AllCabals(context.Context) ([]cabal.View, error) {
	f.all++
	return f.cabals, f.allErr
}

func (f *countingPorts) MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabal.MemberView, error) {
	f.memberCalls++
	return f.members, f.membersErr
}

func (f *countingPorts) ValuationInputsAt(_ context.Context, at time.Time) (treasury.ValuationInputs, error) {
	f.inputsAt = append(f.inputsAt, at)
	f.positions++
	if f.positionsErr != nil {
		return treasury.ValuationInputs{}, f.positionsErr
	}
	f.stakes++
	if f.stakes > f.stakesErrAfter && f.stakesErr != nil {
		return treasury.ValuationInputs{}, f.stakesErr
	}
	f.reservations++
	inputs := treasury.ValuationInputs{Positions: f.positionRows, Stakes: f.stakeRows, Reserved: f.reservedRows}
	return inputs, f.reservedErr
}

func (f *countingPorts) ListingPrices(
	context.Context, []market.ListingKey, time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	f.listingCalls++
	return f.listingRows, f.listingsErr
}

func (f *countingPorts) MemberStakesAt(context.Context, time.Time) ([]treasury.MemberStake, error) {
	f.stakes++
	if f.stakes <= f.stakesErrAfter {
		return f.stakeRows, nil
	}
	return f.stakeRows, f.stakesErr
}

func (f *countingPorts) MemberFlowsBetween(context.Context, time.Time, time.Time) ([]treasury.MemberFlow, error) {
	f.flowCalls++
	return f.flowRows, f.flowsErr
}

func (f *countingPorts) SnapshotsAt(context.Context, sqlc.SnapshotsAtParams) ([]sqlc.SnapshotsAtRow, error) {
	f.snapshotCalls++
	return f.snapshotRows, f.snapshotsErr
}

func (f *countingPorts) PausedCabals(context.Context) (funding.PausedSet, error) {
	f.paused++
	return f.pausedSet, f.pausesErr
}

func (f *countingPorts) AssetsByMints(_ context.Context, mints []string) ([]market.Asset, error) {
	f.assets++
	f.assetMints = append(f.assetMints, mints)
	var held []market.Asset
	for _, asset := range f.assetRows {
		if slices.Contains(mints, asset.Mint.String()) {
			held = append(held, asset)
		}
	}
	return held, f.assetsErr
}

func (f *countingPorts) PricesAsOf(
	_ context.Context,
	assetIDs []market.AssetID,
	at time.Time,
) (map[market.AssetID]market.Price, error) {
	f.asOf++
	f.asOfInstants = append(f.asOfInstants, at)
	if f.asOfAt != nil {
		asked := map[market.AssetID]market.Price{}
		for id, price := range f.asOfAt(at) {
			if slices.Contains(assetIDs, id) {
				asked[id] = price
			}
		}
		return asked, f.asOfErr
	}
	return f.asOfRows, f.asOfErr
}

func (f *countingPorts) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return f.userRows, f.userErr
}

func (f *countingPorts) PreviousEntriesForCabals(
	context.Context,
	sqlc.PreviousEntriesForCabalsParams,
) ([]sqlc.LeaderboardEntry, error) {
	f.previousCalls++
	return f.previousRows, f.previousErr
}

func wantOneReadPerPort(t *testing.T, f *countingPorts) {
	t.Helper()
	once := []int{f.all, f.memberCalls, f.positions, f.paused, f.assets, f.asOf, f.flowCalls, f.snapshotCalls}
	if slices.ContainsFunc(once, func(n int) bool { return n != 1 }) || f.stakes != 1+len(rangedRanges()) {
		t.Fatalf("calls = %#v", f)
	}
}

func twoCabalPorts(t *testing.T) (*countingPorts, chain.SolanaAddress, [2]ids.CabalID) {
	t.Helper()
	usdc, err := chain.ParseAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if err != nil {
		t.Fatal(err)
	}
	f := &countingPorts{}
	var cabalIDs [2]ids.CabalID
	for i := range cabalIDs {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		cabalIDs[i] = id
		f.cabals = append(f.cabals, cabal.View{ID: id})
		f.positionRows = append(f.positionRows, treasury.CabalPositions{
			CabalID:     id,
			Holdings:    []treasury.Position{{Mint: usdc, Units: money.NewBaseUnits(1_000_000_000, 6)}},
			TotalShares: money.SharesUnitsFromUint64(1_000_000_000),
		})
		f.stakeRows = append(f.stakeRows, treasury.MemberStake{
			CabalID: id, UserID: ids.UserIDFrom(ids.Real{}.NewV7()),
			ShareUnits: money.SharesUnitsFromUint64(1_000_000_000),
		})
	}
	return f, usdc, cabalIDs
}

func TestRunValuation_oneCabalWithMoreSharesThanTheTotalDoesNotDropTheOthers(t *testing.T) {
	t.Parallel()
	f, usdc, cabalIDs := twoCabalPorts(t)
	f.stakeRows[1].ShareUnits = money.SharesUnitsFromUint64(1_500_000_000)
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), valuationTime())
	if err != nil {
		t.Fatalf("Run() error = %v, want the inconsistent cabal skipped", err)
	}
	if len(got.Cabals) != 1 || got.Cabals[0].CabalID != cabalIDs[0] || got.Excluded != 1 {
		t.Fatalf("Run() = %#v, want only the consistent cabal valued and one excluded", got)
	}
}

func TestRunValuation_readsEveryTreasuryInputAtTheSameInstantInOneCall(t *testing.T) {
	t.Parallel()
	f, usdc, _ := twoCabalPorts(t)
	at := valuationTime()
	if _, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), at); err != nil {
		t.Fatal(err)
	}
	if len(f.inputsAt) != 1 || !f.inputsAt[0].Equal(at) {
		t.Fatalf("ValuationInputsAt instants = %v, want exactly [%v]", f.inputsAt, at)
	}
}

func TestRunValuation_pricesAsOfIsTheOldestPriceUsed(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 1)
	oldest := valuationTime().Add(-90 * time.Second)
	price := f.asOfRows[f.assetRows[1].ID]
	price.ObservedAt = oldest
	f.asOfRows[f.assetRows[1].ID] = price
	got, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f, Snapshots: f},
		usdc,
	).Run(t.Context(), valuationTime())
	if err != nil || len(got.Cabals) != 1 || !got.AsOf.Equal(valuationTime()) || !got.PricesAsOf.Equal(oldest) {
		t.Fatalf("Run() = AsOf %v PricesAsOf %v cabals %d, %v, want PricesAsOf %v",
			got.AsOf, got.PricesAsOf, len(got.Cabals), err, oldest)
	}
}
