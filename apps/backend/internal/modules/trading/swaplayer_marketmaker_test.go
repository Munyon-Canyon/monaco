package trading_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

const marketMakerSignature = platform.Signature("market-maker-signature")

func (e *layerEnv) scriptMarketMaker() {
	marketMaker := platform.AddressOf(make([]byte, 32))
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()), jupiter.Order{
		RequestID: "req-1", Router: "jupiterz",
		Transaction: chainfake.Unsigned(marketMaker, chainfake.WalletAddress(treasuryWallet)),
	})
}

func (e *layerEnv) executeReturning(result app.ExecuteResult) {
	e.stubExecute(func(context.Context, string, []byte) (app.ExecuteResult, error) {
		if result.Status == app.ExecuteSuccess {
			e.fills.mu.Lock()
			e.fills.landed[result.Signature] = result.OutAmount
			e.fills.mu.Unlock()
		}
		return result, nil
	})
}

func TestSwapLayer_aMarketMakerRouteIsSentAndKeepsTheSignatureJupiterReturns(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.scriptMarketMaker()
	e.executeReturning(app.ExecuteResult{
		Status: app.ExecuteSuccess, Signature: marketMakerSignature, OutAmount: 105_000_000,
	})
	got := e.settled(t, e.request(e.source()))
	if got.Status != domain.StatusConfirmed || got.OutAmount != 105_000_000 {
		t.Fatalf("view = %+v, want confirmed", got)
	}
	_, signed, _, signature := e.row(t, got.ID.UUID())
	if signature == nil || platform.Signature(*signature) != marketMakerSignature {
		t.Fatalf("stored tx_signature = %v, want %q from /execute", signature, marketMakerSignature)
	}
	if len(signed) == 0 {
		t.Fatal("the signed transaction was not stored before execute")
	}
	if p := e.payload(t, got.ID.UUID(), "trade.confirmed"); p["tx_signature"] != string(marketMakerSignature) {
		t.Fatalf("trade.confirmed tx_signature = %v, want %q", p["tx_signature"], marketMakerSignature)
	}
}

func TestSwapLayer_aMarketMakerRouteKeepsAStandInWhenExecuteGivesNoSignature(t *testing.T) {
	t.Parallel()
	for name, result := range map[string]app.ExecuteResult{
		"pending without a signature": {Status: app.ExecutePending},
		"failed without a signature":  {Status: app.ExecuteFailed, ErrorCode: -2000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newLayerEnv(t)
			e.scriptMarketMaker()
			e.executeReturning(result)
			got := e.settled(t, e.request(e.source()))
			if got.Status != domain.StatusSubmitted {
				t.Fatalf("status = %s, want submitted", got.Status)
			}
			if _, _, _, signature := e.row(t, got.ID.UUID()); signature == nil ||
				!platform.IsStandInSignature(platform.Signature(*signature)) {
				t.Fatalf("stored tx_signature = %v, want a stand-in", signature)
			}
		})
	}
}

func TestSwapLayer_aMarketMakerRouteThatIsStillPendingSavesTheSignatureJupiterReturned(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.scriptMarketMaker()
	e.executeReturning(app.ExecuteResult{Status: app.ExecutePending, Signature: marketMakerSignature})
	got := e.settled(t, e.request(e.source()))
	if got.Status != domain.StatusSubmitted {
		t.Fatalf("status = %s, want submitted", got.Status)
	}
	if _, _, _, signature := e.row(t, got.ID.UUID()); signature == nil ||
		platform.Signature(*signature) != marketMakerSignature {
		t.Fatalf("stored tx_signature = %v, want %q so the sweeper can follow it", signature, marketMakerSignature)
	}
}

func TestSwapLayer_aSignatureThatCannotBeSavedLeavesTheStandInAndSurfaces(t *testing.T) {
	t.Parallel()
	e := newLayerEnv(t)
	e.scriptMarketMaker()
	e.executeReturning(app.ExecuteResult{Status: app.ExecutePending, Signature: marketMakerSignature})
	const refuse = `
		CREATE FUNCTION refuse_adopt() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.tx_signature LIKE 'unsent:%' THEN RAISE EXCEPTION 'adopt refused'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER refuse_adopt BEFORE UPDATE OF tx_signature ON swaps
		FOR EACH ROW EXECUTE FUNCTION refuse_adopt()`
	if _, err := e.pool.Exec(t.Context(), refuse); err != nil {
		t.Fatal(err)
	}
	req := e.request(e.source())
	if _, err := e.run(t, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Run = %v, want internal", err)
	}
	_, _, _, signature := e.row(t, e.swapIDs(t, req.Source.ID)[0])
	if signature == nil || !platform.IsStandInSignature(platform.Signature(*signature)) {
		t.Fatalf("stored tx_signature = %v, want the stand-in kept", signature)
	}
}
