package multichain

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type (
	listing struct{}
	stock   struct{}
)

type (
	ListingID = ids.ID[listing]
	StockID   = ids.ID[stock]
)

type Venue string

const (
	VenueJupiter Venue = "jupiter"
	VenueUniswap Venue = "uniswap"
)

type Listing struct {
	ID             ListingID     `json:"id"`
	StockID        StockID       `json:"stock_id"`
	Issuer         string        `json:"issuer"`
	Symbol         string        `json:"symbol"`
	Network        chain.Network `json:"network"`
	TokenAddress   string        `json:"token_address"`
	WrapperAddress string        `json:"wrapper_address,omitempty"`
	Decimals       uint8         `json:"decimals"`
	SharesPerToken *big.Rat      `json:"shares_per_token"`
	Venue          Venue         `json:"venue"`
}

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type CostKind string

const (
	CostPriceImpact CostKind = "price_impact"
	CostPoolFee     CostKind = "pool_fee"
	CostBridge      CostKind = "bridge"
	CostGas         CostKind = "gas"
	CostPrivyFee    CostKind = "privy_fee"
)

type Cost struct {
	Kind   CostKind     `json:"kind"`
	Micros money.Micros `json:"micros"`
}

type ShareNanos uint64

type Quote struct {
	ListingID   ListingID     `json:"listing_id"`
	Network     chain.Network `json:"network"`
	Venue       Venue         `json:"venue"`
	Side        Side          `json:"side"`
	InputMicros money.Micros  `json:"input_micros"`
	GrossOutput *big.Int      `json:"gross_output_units"`
	Costs       []Cost        `json:"costs"`
	NetShares   ShareNanos    `json:"net_share_nanos"`
	NetMicros   money.Micros  `json:"net_micros"`
	QuotedAt    time.Time     `json:"quoted_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	RouteID     string        `json:"route_id"`
}

type Comparison struct {
	StockID StockID   `json:"stock_id"`
	Side    Side      `json:"side"`
	Ranked  []Quote   `json:"ranked"`
	Winner  ListingID `json:"winner"`
}

type Price struct {
	PerShareMicros money.Micros `json:"per_share_micros"`
	AsOf           time.Time    `json:"as_of"`
}

type ActionID string

type ActionState string

const (
	ActionPending   ActionState = "pending"
	ActionSucceeded ActionState = "succeeded"
	ActionRejected  ActionState = "rejected"
	ActionFailed    ActionState = "failed"
)

type StepState string

const (
	StepPending   StepState = "pending"
	StepConfirmed StepState = "confirmed"
	StepFailed    StepState = "failed"
)

type ActionStep struct {
	Network chain.Network `json:"network"`
	TxHash  string        `json:"tx_hash,omitempty"`
	State   StepState     `json:"state"`
}

type ActionStatus struct {
	ID    ActionID     `json:"id"`
	State ActionState  `json:"state"`
	Steps []ActionStep `json:"steps"`
}

type TransferSpec struct {
	From         chain.Network `json:"from"`
	To           chain.Network `json:"to"`
	Destination  string        `json:"destination"`
	AmountMicros money.Micros  `json:"amount_micros"`
}

type WalletActions interface {
	Transfer(ctx context.Context, walletID string, spec TransferSpec, idemKey string) (ActionID, error)
	Action(ctx context.Context, walletID string, id ActionID) (ActionStatus, error)
}

type EVMWallet struct {
	ID      string           `json:"id"`
	Address chain.EVMAddress `json:"address"`
}

type EVMTransaction struct {
	Network chain.Network    `json:"network"`
	To      chain.EVMAddress `json:"to"`
	Data    []byte           `json:"data"`
	Value   *big.Int         `json:"value"`
}

type EVMWallets interface {
	CreateAppEVMWallet(ctx context.Context, key string) (EVMWallet, error)
	SignEVMTransaction(ctx context.Context, walletID string, tx EVMTransaction, sponsor bool) ([]byte, error)
	SendEVMTransaction(ctx context.Context, walletID string, tx EVMTransaction, sponsor bool) (string, error)
}

type AutomationID string

type Execution struct {
	ID           string        `json:"id"`
	ActionID     ActionID      `json:"action_id"`
	Network      chain.Network `json:"network"`
	SourceTx     string        `json:"source_tx"`
	AmountMicros money.Micros  `json:"amount_micros"`
	State        ActionState   `json:"state"`
	Signature    string        `json:"solana_signature,omitempty"`
}

type ExecutionPage struct {
	Executions []Execution `json:"executions"`
	Next       string      `json:"next,omitempty"`
}

type DepositForwarding interface {
	CreateAutomation(ctx context.Context) (AutomationID, error)
	Attach(ctx context.Context, evmWalletID string, solanaDest chain.SolanaAddress) error
	Executions(ctx context.Context, walletID, cursor string) (ExecutionPage, error)
	Reindex(ctx context.Context, walletID string, network chain.Network) error
}

type Quoter interface {
	Quote(ctx context.Context, l Listing, side Side, amountMicros money.Micros) (Quote, error)
}

type Router interface {
	Best(ctx context.Context, stockID StockID, side Side, amountMicros money.Micros) (Comparison, error)
}

type EVMPriceSource interface {
	Prices(ctx context.Context, listings []Listing) (map[ListingID]Price, error)
}

type Pending struct{ Ticket string }

func (p Pending) notImplemented(method string) error {
	return errs.New(errs.CodeNotImplemented, "multichain."+method, slog.String("ticket", p.Ticket))
}

func (p Pending) Transfer(context.Context, string, TransferSpec, string) (ActionID, error) {
	return "", p.notImplemented("Transfer")
}

func (p Pending) Action(context.Context, string, ActionID) (ActionStatus, error) {
	return ActionStatus{}, p.notImplemented("Action")
}

func (p Pending) CreateAppEVMWallet(context.Context, string) (EVMWallet, error) {
	return EVMWallet{}, p.notImplemented("CreateAppEVMWallet")
}

func (p Pending) SignEVMTransaction(context.Context, string, EVMTransaction, bool) ([]byte, error) {
	return nil, p.notImplemented("SignEVMTransaction")
}

func (p Pending) SendEVMTransaction(context.Context, string, EVMTransaction, bool) (string, error) {
	return "", p.notImplemented("SendEVMTransaction")
}

func (p Pending) CreateAutomation(context.Context) (AutomationID, error) {
	return "", p.notImplemented("CreateAutomation")
}

func (p Pending) Attach(context.Context, string, chain.SolanaAddress) error {
	return p.notImplemented("Attach")
}

func (p Pending) Executions(context.Context, string, string) (ExecutionPage, error) {
	return ExecutionPage{}, p.notImplemented("Executions")
}

func (p Pending) Reindex(context.Context, string, chain.Network) error {
	return p.notImplemented("Reindex")
}

func (p Pending) Quote(context.Context, Listing, Side, money.Micros) (Quote, error) {
	return Quote{}, p.notImplemented("Quote")
}

func (p Pending) Best(context.Context, StockID, Side, money.Micros) (Comparison, error) {
	return Comparison{}, p.notImplemented("Best")
}

func (p Pending) Prices(context.Context, []Listing) (map[ListingID]Price, error) {
	return nil, p.notImplemented("Prices")
}
