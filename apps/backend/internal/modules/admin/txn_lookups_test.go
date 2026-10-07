package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	txnSignature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"
	explorerBase = "https://solscan.io/tx/"
)

func (f lookupFixture) swap(t *testing.T, cabal uuid.UUID, status, signature, requestID string) uuid.UUID {
	t.Helper()
	id := f.ids.NewV7()
	f.exec(t, `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address, action, symbol, in_mint,
		out_mint, out_decimals, in_amount, slippage_bps, source_batch_size, status, failure_code, execute_request_id,
		tx_signature, created_at, submitted_at, updated_at, out_amount, confirmed_at)
		VALUES ($1, 'proposal', $2, $3, 'treasury', 'buy', 'AAPLx', 'in', 'out', 8, 25000000, 100, 1, $4,
		CASE WHEN $4 = 'failed' THEN 'jupiter_failed' END, NULLIF($5, ''), NULLIF($6, ''), $7, $7, $7,
		CASE WHEN $4 = 'confirmed' THEN 104900000 END, CASE WHEN $4 = 'confirmed' THEN $7::timestamptz END)`,
		id, f.ids.NewV7(), cabal, status, requestID, signature, f.clock.Now())
	return id
}

func (f lookupFixture) swapEvent(t *testing.T, swap uuid.UUID, eventType string, at time.Time) {
	t.Helper()
	f.exec(t, `INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'swap', $2, $3, '{"v":1}', 'system', 'test', $4)`, f.ids.NewV7(), swap, eventType, at)
}

func (f lookupFixture) ledgerSwap(t *testing.T, cabal, swap uuid.UUID, signature string) uuid.UUID {
	t.Helper()
	id := f.ids.NewV7()
	f.exec(t, `INSERT INTO cabal_txns (id, cabal_id, kind, status, swap_id, tx_signature, created_at, seq)
		VALUES ($1, $2, 'swap', 'settled', $3, $4, $5, 1)`, id, cabal, swap, signature, f.clock.Now())
	f.exec(t, `INSERT INTO cabal_txn_entries (txn_id, seq, account, asset, amount) VALUES
		($1, 0, 'treasury', 'usdc', -5000000), ($1, 1, 'venue', 'usdc', 5000000)`, id)
	return id
}

func (f lookupFixture) txn(t *testing.T, path string) adminapi.AdminTxn {
	t.Helper()
	w := adminRequest(t, f.h, path, "viewer")
	var body adminapi.AdminTxn
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s (%v)", path, w.Code, w.Body, err)
	}
	return body
}

func TestAdminLookup_Txn_BySignature_ExplorerLink(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	swap := f.swap(t, c.ID.UUID(), "confirmed", txnSignature, "req-1")
	ledger := f.ledgerSwap(t, c.ID.UUID(), swap, txnSignature)
	body := f.txn(t, "/v1/admin/txns?signature="+txnSignature)
	if body.ExplorerUrl == nil || *body.ExplorerUrl != explorerBase+txnSignature {
		t.Fatalf("explorer_url = %v", body.ExplorerUrl)
	}
	if body.Ledger == nil || body.Ledger.Id != ledger || body.Ledger.Kind != "swap" || len(body.Ledger.Entries) != 2 {
		t.Fatalf("ledger = %+v", body.Ledger)
	}
	if entry := body.Ledger.Entries[0]; entry.Account != "treasury" || entry.Amount != "-5000000" {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestAdminLookup_Txn_ByLedgerID_LinksTheSwapAndItsHistory(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	swap := f.swap(t, c.ID.UUID(), "confirmed", txnSignature, "req-1")
	ledger := f.ledgerSwap(t, c.ID.UUID(), swap, txnSignature)
	at := f.clock.Now()
	f.swapEvent(t, swap, "trade.submitted", at)
	f.swapEvent(t, swap, "trade.confirmed", at.Add(time.Minute))
	f.swapEvent(t, swap, "trade.blocked", at.Add(2*time.Minute))
	body := f.txn(t, "/v1/admin/txns/"+ledger.String())
	if body.Swap == nil || body.Swap.Id != swap || body.Swap.Status != "confirmed" ||
		*body.Swap.ExecuteRequestId != "req-1" || *body.Swap.TxSignature != txnSignature {
		t.Fatalf("swap = %+v", body.Swap)
	}
	history := body.Swap.StatusHistory
	if len(history) != 2 || history[0].Type != "trade.submitted" || history[1].Type != "trade.confirmed" ||
		!history[1].At.Equal(at.Add(time.Minute)) {
		t.Fatalf("history = %+v", history)
	}
}

func TestAdminLookup_Txn_BySwapID_ForAStuckSwapWithoutALedgerRow(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	swap := f.swap(t, c.ID.UUID(), "created", "", "")
	body := f.txn(t, "/v1/admin/txns/"+swap.String())
	if body.Ledger != nil || body.ExplorerUrl != nil || body.Swap == nil || body.Swap.Status != "created" ||
		body.Swap.ExecuteRequestId != nil || body.Swap.TxSignature != nil {
		t.Fatalf("body = %+v", body)
	}
	failed := f.swap(t, c.ID.UUID(), "failed", "sig-failed", "req-2")
	if got := f.txn(t, "/v1/admin/txns/"+failed.String()); *got.Swap.FailureCode != "jupiter_failed" {
		t.Fatalf("failed swap = %+v", got.Swap)
	}
	bySignature := f.txn(t, "/v1/admin/txns?signature=sig-failed")
	if bySignature.Swap == nil || bySignature.Swap.Id != failed || bySignature.Ledger != nil {
		t.Fatalf("by signature = %+v", bySignature)
	}
}

func TestAdminLookup_Txn_UserLedgerRowWithoutASwap(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "alice"})
	id := f.ids.NewV7()
	f.exec(t, `INSERT INTO user_txns (id, user_id, kind, status, created_at) VALUES ($1, $2, 'deposit', 'settled', $3)`,
		id, user.ID.UUID(), f.clock.Now())
	body := f.txn(t, "/v1/admin/txns/"+id.String())
	if body.Swap != nil || body.ExplorerUrl != nil || body.Ledger == nil || body.Ledger.UserId == nil ||
		*body.Ledger.UserId != user.ID.UUID() || len(body.Ledger.Entries) != 0 {
		t.Fatalf("body = %+v", body)
	}
}

func TestAdminLookup_Txn_MissingIsNotFound(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	for _, path := range []string{"/v1/admin/txns/" + f.ids.NewV7().String(), "/v1/admin/txns?signature=nope"} {
		w := adminRequest(t, f.h, path, "viewer")
		var problem map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil || w.Code != http.StatusNotFound ||
			problem["code"] != string(errs.CodeTxnNotFound) {
			t.Errorf("GET %s = %d %s", path, w.Code, w.Body)
		}
	}
}

func TestAdminLookup_Txn_QueryCount(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	swap := f.swap(t, c.ID.UUID(), "confirmed", txnSignature, "req-1")
	ledger := f.ledgerSwap(t, c.ID.UUID(), swap, txnSignature)
	f.swapEvent(t, swap, "trade.submitted", f.clock.Now())
	for name, path := range map[string]string{
		"admin GetAdminTxn":  "/v1/admin/txns/" + ledger.String(),
		"admin FindAdminTxn": "/v1/admin/txns?signature=" + txnSignature,
	} {
		testkit.AssertQueries(t, name, func() {
			if w := adminRequest(t, f.h, path, "viewer"); w.Code != http.StatusOK {
				t.Fatalf("status = %d %s", w.Code, w.Body)
			}
		})
	}
}
