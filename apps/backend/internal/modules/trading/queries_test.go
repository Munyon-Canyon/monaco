package trading_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type portDB struct {
	swapDB
	port trading.Queries
}

func newPortDB(t *testing.T) portDB {
	t.Helper()
	d := newSwapDB(t)
	return portDB{swapDB: d, port: trading.New(module.Deps{Pool: d.pool}).Queries()}
}

func proposal(id uuid.UUID) trading.Source {
	return trading.Source{Kind: domain.SourceProposal, ID: id}
}

func TestQueries_swapReadsEveryFieldOfTheRow(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	id := ids.SwapIDFrom(row.ID)
	want := trading.SwapView{
		ID: id, CabalID: ids.CabalIDFrom(row.CabalID), Source: proposal(row.SourceID), Action: domain.ActionBuy,
		Symbol: "AAPLx", InAmount: 25_000_000, OutDecimals: 8, Status: domain.StatusCreated, CreatedAt: d.now,
	}
	got, err := d.port.Swap(t.Context(), id)
	if err != nil || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("Swap = %+v, %v", got, err)
	}
	got.CreatedAt = want.CreatedAt
	if got != want {
		t.Fatalf("Swap = %+v, want %+v", got, want)
	}
	d.submit(t, row.ID, "req-1", "sig-1")
	d.confirm(t, row.ID)
	got, err = d.port.SwapBySignature(t.Context(), "sig-1")
	if err != nil || got.ID != id || got.Status != domain.StatusConfirmed || got.OutAmount != 104_900_000 ||
		got.OutDecimals != 8 || got.TxSignature != "sig-1" || !got.ConfirmedAt.Equal(d.now) || got.Retryable {
		t.Fatalf("SwapBySignature after confirm = %+v, %v", got, err)
	}
}

func TestQueries_missingSwapsAreSwapNotFound(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	if _, err := d.port.Swap(t.Context(), ids.SwapIDFrom(d.ids.NewV7())); errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("Swap of an unknown id err = %v, want swap_not_found", err)
	}
	if _, err := d.port.SwapBySignature(t.Context(), "nope"); errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("SwapBySignature of an unknown signature err = %v, want swap_not_found", err)
	}
	if v, ok, err := d.port.LatestBySource(t.Context(), proposal(d.ids.NewV7())); ok || err != nil ||
		v != (trading.SwapView{}) {
		t.Fatalf("LatestBySource of an unknown source = %+v, %t, %v, want nothing", v, ok, err)
	}
}

func TestQueries_latestBySourceIsRetryableOnlyWhenItFailedLast(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	source := d.ids.NewV7()
	first := d.created(source, usdcMint)
	d.insert(t, first)
	d.fail(t, first.ID, string(domain.FailureNeverSubmitted))
	latest, ok, err := d.port.LatestBySource(t.Context(), proposal(source))
	if err != nil || !ok || latest.ID != ids.SwapIDFrom(first.ID) || !latest.Retryable ||
		latest.FailureCode != domain.FailureNeverSubmitted {
		t.Fatalf("LatestBySource after one failure = %+v, %t, %v, want the retryable failed row", latest, ok, err)
	}
	retry := d.created(source, usdcMint)
	retry.CreatedAt = d.now.Add(time.Minute)
	d.insert(t, retry)
	latest, ok, err = d.port.LatestBySource(t.Context(), proposal(source))
	if err != nil || !ok || latest.ID != ids.SwapIDFrom(retry.ID) || latest.Retryable {
		t.Fatalf("LatestBySource after the retry = %+v, %t, %v, want the retry", latest, ok, err)
	}
	old, err := d.port.Swap(t.Context(), ids.SwapIDFrom(first.ID))
	if err != nil || old.Retryable {
		t.Fatalf("Swap of the superseded failure = %+v, %v, want it no longer retryable", old, err)
	}
}

func TestQueries_retryableIsScopedToTheMintOfTheFailedSwap(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	source := d.ids.NewV7()
	failed := d.created(source, aaplxMint)
	d.insert(t, failed)
	d.fail(t, failed.ID, string(domain.FailureNeverSubmitted))
	other := d.created(source, tslaxMint)
	other.CreatedAt = d.now.Add(time.Minute)
	d.insert(t, other)
	got, err := d.port.Swap(t.Context(), ids.SwapIDFrom(failed.ID))
	if err != nil || !got.Retryable {
		t.Fatalf("Swap of a failure with a later swap on another mint = %+v, %v, want retryable", got, err)
	}
	retry := d.created(source, aaplxMint)
	retry.CreatedAt = d.now.Add(2 * time.Minute)
	d.insert(t, retry)
	if got, err = d.port.Swap(t.Context(), ids.SwapIDFrom(failed.ID)); err != nil || got.Retryable {
		t.Fatalf("Swap of a failure with a later swap on the same mint = %+v, %v, want not retryable", got, err)
	}
}

func TestQueries_hasLiveSwapForCreatedSubmittedAndConfirmedOnly(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	for _, status := range domain.Statuses() {
		id := d.at(t, status)
		var source uuid.UUID
		if err := d.pool.QueryRow(t.Context(), `SELECT source_id FROM swaps WHERE id = $1`, id).
			Scan(&source); err != nil {
			t.Fatal(err)
		}
		live, err := d.port.HasLiveSwap(t.Context(), proposal(source))
		if want := status != domain.StatusFailed; err != nil || live != want {
			t.Errorf("HasLiveSwap with one %s row = %t, %v, want %t", status, live, err, want)
		}
	}
	if live, err := d.port.HasLiveSwap(t.Context(), proposal(d.ids.NewV7())); err != nil || live {
		t.Fatalf("HasLiveSwap of an unknown source = %t, %v, want false", live, err)
	}
}

func TestQueries_ownsSignatureOnlyForItsOwnSwaps(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	d.submit(t, row.ID, "req-1", "sig-1")
	for sig, want := range map[chain.Signature]bool{"sig-1": true, "sig-2": false, "": false} {
		if owns, err := d.port.OwnsSignature(t.Context(), sig); err != nil || owns != want {
			t.Errorf("OwnsSignature(%q) = %t, %v, want %t", sig, owns, err, want)
		}
	}
}

func (d portDB) corrupt(t *testing.T, set string) uuid.UUID {
	t.Helper()
	row := d.created(d.ids.NewV7(), usdcMint)
	d.insert(t, row)
	for _, stmt := range []string{
		`ALTER TABLE swaps DROP CONSTRAINT swaps_status_check, DROP CONSTRAINT swaps_source_kind_check,
			DROP CONSTRAINT swaps_failure_code_check`,
		`UPDATE swaps SET ` + set + ` WHERE id = $1`,
	} {
		args := []any{}
		if strings.Contains(stmt, "$1") {
			args = append(args, row.ID)
		}
		if _, err := d.pool.Exec(t.Context(), stmt, args...); err != nil {
			t.Fatal(err)
		}
	}
	return row.ID
}

func TestQueries_reportAnUnreadableRowAsDecodeFailed(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]string{
		"status":       `status = 'lost'`,
		"source kind":  `source_kind = 'unplanned_kind'`,
		"failure code": `status = 'failed', failure_code = 'gremlins'`,
		"in amount":    `in_amount = -1`,
		"out amount":   `out_amount = -1`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newPortDB(t)
			id := d.corrupt(t, set)
			if _, err := d.port.Swap(t.Context(), ids.SwapIDFrom(id)); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Errorf("Swap err = %v, want decode_failed", err)
			}
			var kind string
			var source uuid.UUID
			if err := d.pool.QueryRow(t.Context(), `SELECT source_kind, source_id FROM swaps WHERE id = $1`, id).
				Scan(&kind, &source); err != nil {
				t.Fatal(err)
			}
			src := trading.Source{Kind: domain.SourceKind(kind), ID: source}
			if v, ok, err := d.port.LatestBySource(t.Context(), src); ok || errs.CodeOf(err) != errs.CodeDecodeFailed ||
				v != (trading.SwapView{}) {
				t.Errorf("LatestBySource = %+v, %t, %v, want decode_failed", v, ok, err)
			}
		})
	}
}

func TestQueries_reportANullThatTheStatusRequiresAsDecodeFailed(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		set    string
		decode bool
	}{
		"created needs nothing":         {`status = 'created'`, true},
		"submitted with a signature":    {`status = 'submitted', tx_signature = 'sig'`, true},
		"submitted without a signature": {`status = 'submitted'`, false},
		"confirmed without a signature": {
			`status = 'confirmed', out_amount = 1, confirmed_at = now()`, false,
		},
		"confirmed without an out amount": {`status = 'confirmed', tx_signature = 'sig', confirmed_at = now()`, false},
		"confirmed without a time":        {`status = 'confirmed', tx_signature = 'sig', out_amount = 1`, false},
		"failed without a code":           {`status = 'failed', failed_at = now()`, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newPortDB(t)
			id := d.corrupt(t, c.set)
			_, err := d.port.Swap(t.Context(), ids.SwapIDFrom(id))
			if c.decode != (err == nil) || (err != nil && errs.CodeOf(err) != errs.CodeDecodeFailed) {
				t.Errorf("Swap err = %v, want readable %t", err, c.decode)
			}
		})
	}
}

func TestQueries_reportADatabaseFailureAsInternal(t *testing.T) {
	t.Parallel()
	d := newPortDB(t)
	if _, err := d.pool.Exec(t.Context(), `DROP TABLE swaps CASCADE`); err != nil {
		t.Fatal(err)
	}
	src := proposal(d.ids.NewV7())
	_, swapErr := d.port.Swap(t.Context(), ids.SwapIDFrom(d.ids.NewV7()))
	_, sigErr := d.port.SwapBySignature(t.Context(), "sig-1")
	_, _, latestErr := d.port.LatestBySource(t.Context(), src)
	_, liveErr := d.port.HasLiveSwap(t.Context(), src)
	_, ownsErr := d.port.OwnsSignature(t.Context(), "sig-1")
	for name, err := range map[string]error{
		"Swap": swapErr, "SwapBySignature": sigErr, "LatestBySource": latestErr, "HasLiveSwap": liveErr,
		"OwnsSignature": ownsErr,
	} {
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s without the table err = %v, want internal", name, err)
		}
	}
}
