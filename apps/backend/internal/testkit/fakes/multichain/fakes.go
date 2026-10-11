package multichain

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type WalletActions struct {
	Transfers *Script[mc.ActionID]
	Actions   *Script[mc.ActionStatus]
}

func (f *WalletActions) Transfer(context.Context, string, mc.TransferSpec, string) (mc.ActionID, error) {
	r := f.Transfers.next()
	return r.Value, r.err("fakes.WalletActions.Transfer")
}

func (f *WalletActions) Action(context.Context, string, mc.ActionID) (mc.ActionStatus, error) {
	r := f.Actions.next()
	return r.Value, r.err("fakes.WalletActions.Action")
}

type EVMWallets struct {
	Creates *Script[mc.EVMWallet]
	Signs   *Script[[]byte]
	Sends   *Script[string]
}

func (f *EVMWallets) CreateAppEVMWallet(context.Context, string) (mc.EVMWallet, error) {
	r := f.Creates.next()
	return r.Value, r.err("fakes.EVMWallets.CreateAppEVMWallet")
}

func (f *EVMWallets) SignEVMTransaction(context.Context, string, mc.EVMTransaction, bool) ([]byte, error) {
	r := f.Signs.next()
	return r.Value, r.err("fakes.EVMWallets.SignEVMTransaction")
}

func (f *EVMWallets) SendEVMTransaction(context.Context, string, mc.EVMTransaction, bool) (string, error) {
	r := f.Sends.next()
	return r.Value, r.err("fakes.EVMWallets.SendEVMTransaction")
}

type DepositForwarding struct {
	Automations *Script[mc.AutomationID]
	Attaches    *Script[struct{}]
	Pages       *Script[mc.ExecutionPage]
	Reindexes   *Script[struct{}]
}

func (f *DepositForwarding) CreateAutomation(context.Context) (mc.AutomationID, error) {
	r := f.Automations.next()
	return r.Value, r.err("fakes.DepositForwarding.CreateAutomation")
}

func (f *DepositForwarding) Attach(context.Context, string, chain.SolanaAddress) error {
	return f.Attaches.next().err("fakes.DepositForwarding.Attach")
}

func (f *DepositForwarding) Executions(context.Context, string, string) (mc.ExecutionPage, error) {
	r := f.Pages.next()
	return r.Value, r.err("fakes.DepositForwarding.Executions")
}

func (f *DepositForwarding) Reindex(context.Context, string, chain.Network) error {
	return f.Reindexes.next().err("fakes.DepositForwarding.Reindex")
}

type Quoter struct{ Quotes *Script[mc.Quote] }

func (f *Quoter) Quote(_ context.Context, l mc.Listing, side mc.Side, amount money.Micros) (mc.Quote, error) {
	r := f.Quotes.next()
	if err := r.err("fakes.Quoter.Quote"); err != nil {
		return mc.Quote{}, err
	}
	q := r.Value
	q.ListingID, q.Network, q.Venue, q.Side, q.InputMicros = l.ID, l.Network, l.Venue, side, amount
	return q, nil
}

type Router struct{ Comparisons *Script[mc.Comparison] }

func (f *Router) Best(_ context.Context, stockID mc.StockID, side mc.Side, _ money.Micros) (mc.Comparison, error) {
	r := f.Comparisons.next()
	if err := r.err("fakes.Router.Best"); err != nil {
		return mc.Comparison{}, err
	}
	c := r.Value
	c.StockID, c.Side = stockID, side
	return c, nil
}

type EVMPriceSource struct {
	Replies *Script[map[mc.ListingID]mc.Price]
}

func (f *EVMPriceSource) Prices(context.Context, []mc.Listing) (map[mc.ListingID]mc.Price, error) {
	r := f.Replies.next()
	return r.Value, r.err("fakes.EVMPriceSource.Prices")
}

type Fakes struct {
	WalletActions     *WalletActions
	EVMWallets        *EVMWallets
	DepositForwarding *DepositForwarding
	Quoter            *Quoter
	Router            *Router
	EVMPriceSource    *EVMPriceSource
}

func New() *Fakes {
	return &Fakes{
		WalletActions: &WalletActions{
			Transfers: recorded[mc.ActionID]("wallet_actions_transfer"),
			Actions:   recorded[mc.ActionStatus]("wallet_actions_action"),
		},
		EVMWallets: &EVMWallets{
			Creates: recorded[mc.EVMWallet]("evm_wallets_create"),
			Signs:   recorded[[]byte]("evm_wallets_sign"),
			Sends:   recorded[string]("evm_wallets_send"),
		},
		DepositForwarding: &DepositForwarding{
			Automations: recorded[mc.AutomationID]("deposit_forwarding_automation"),
			Attaches:    recorded[struct{}]("deposit_forwarding_attach"),
			Pages:       recorded[mc.ExecutionPage]("deposit_forwarding_executions"),
			Reindexes:   recorded[struct{}]("deposit_forwarding_reindex"),
		},
		Quoter:         &Quoter{Quotes: recorded[mc.Quote]("quoter_quote")},
		Router:         &Router{Comparisons: recorded[mc.Comparison]("router_best")},
		EVMPriceSource: &EVMPriceSource{Replies: recorded[map[mc.ListingID]mc.Price]("evm_prices")},
	}
}

type script interface {
	PushJSON(raw []byte) error
	NextJSON() ([]byte, error)
}

func (f *Fakes) scripts() map[string]script {
	return map[string]script{
		"wallet_actions_transfer":       f.WalletActions.Transfers,
		"wallet_actions_action":         f.WalletActions.Actions,
		"evm_wallets_create":            f.EVMWallets.Creates,
		"evm_wallets_sign":              f.EVMWallets.Signs,
		"evm_wallets_send":              f.EVMWallets.Sends,
		"deposit_forwarding_automation": f.DepositForwarding.Automations,
		"deposit_forwarding_attach":     f.DepositForwarding.Attaches,
		"deposit_forwarding_executions": f.DepositForwarding.Pages,
		"deposit_forwarding_reindex":    f.DepositForwarding.Reindexes,
		"quoter_quote":                  f.Quoter.Quotes,
		"router_best":                   f.Router.Comparisons,
		"evm_prices":                    f.EVMPriceSource.Replies,
	}
}

func (f *Fakes) Mount(mux *http.ServeMux) {
	scripts := f.scripts()
	named := func(w http.ResponseWriter, r *http.Request) (script, bool) {
		s, ok := scripts[r.PathValue("script")]
		if !ok {
			http.Error(w, "unknown multichain script", http.StatusNotFound)
		}
		return s, ok
	}
	mux.HandleFunc("POST /_multichain/{script}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := named(w, r)
		if !ok {
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err == nil {
			err = s.PushJSON(raw)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /multichain/{script}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := named(w, r)
		if !ok {
			return
		}
		raw, err := s.NextJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("GET /_multichain", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(slices.Sorted(maps.Keys(scripts)))
	})
}
